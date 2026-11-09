import {
    type InfiniteData,
    keepPreviousData,
    type QueryKey,
    useInfiniteQuery,
    useQueryClient,
} from '@tanstack/react-query'
import { useCallback, useRef, useState } from 'react'
import { mergePages } from '../lib/rows'
import {
    foldKeyOf,
    groupRowAt,
    isTransientRunError,
    LoadMoreRefused,
    lastGroupRowOf,
    pageRowCount,
    RUN_QUERY_KEY,
    type RunBody,
    type RunPage,
    rowsKeyOf,
    runFirstPage,
    runReport,
    withPage,
} from '../lib/run-client'
import { type RunView, runView } from '../lib/run-state'
import type { ResultRow, RunResult } from '../lib/types'
import { useReportRecentStore } from '../stores/report-recent-store'
import { useLiveTopics } from './useLiveTopics'

export interface ReportRun extends Omit<RunView, 'topics' | 'heldRows'> {
    /** Optional, because a hand-built ReportRun (a mockup) holds nothing back. */
    heldRows?: readonly ResultRow[]
    /**
     * The rows key of the body whose pages are shown. While a changed run loads, the
     * previous body's pages stay in view, so this changes when the new run lands.
     * It is optional because a hand-built ReportRun (a mockup) has no run body.
     * Without it, a design change does not reset the list to the top.
     */
    shownRowsKey?: string
    /**
     * The list reports the index of the row at the top of its view here (in the
     * result's rows). A fold run keeps the group row at or above that row, and loads
     * no further. Optional, because a hand-built ReportRun (a mockup) has no pages
     * to fold.
     */
    onTopRowIndex?(index: number): void
    hasMore: boolean
    isLoadingMore: boolean
    loadMore(): void
    /**
     * The next page failed (R8): the rows shown stay, and the list ends in a row with
     * Retry. Optional, because a hand-built ReportRun (a mockup) loads no pages.
     */
    loadMoreFailed?: boolean
    /** Asks again for the next page that failed. Optional, as `loadMoreFailed`. */
    retryLoadMore?(): void
    retry(): void
}

const resultsOf = (pages: readonly RunPage[]) =>
    pages.flatMap(page => (page.kind === 'ok' ? [page.result] : []))

/**
 * How far a run's first page loads: its rows as a page's limit counts them (group,
 * opening, detail and subtotal rows, J1a), and the group row it must hold (the
 * group at or above the row at the top of the view).
 */
type ReaderPlace = { rows: number; groupRow?: string }

const pageRowsOf = (results: readonly RunResult[]) =>
    results.reduce((n, result) => n + pageRowCount(result), 0)

/** The pages in view: the body they answer (its fold key) and their results. */
type ShownPages = { foldKey: string; results: RunResult[] }

const NO_PLACE: ReaderPlace = { rows: 0 }

const rowIdAt = (result: RunResult, index: number | undefined) =>
    index === undefined ? undefined : result.rows[index]?.id

// Runs a report through POST /api/reports/run (R9). Every report pages by keyset; a
// grouped one pages its detail rows in document order with the section rows between
// them (mergePages joins the pages). The result's topics keep it live.
export function useReportRun({
    body,
    userId,
}: {
    body: RunBody | null
    userId: string
}): ReportRun {
    const queryClient = useQueryClient()
    // Read by the query function when a new run starts, so it is the pages that
    // were in view then (keepPreviousData keeps them until the new run lands).
    const shown = useRef<ShownPages | null>(null)
    // The index of the row at the top of the list's view, as the list last reported
    // it. An index, not a row id: the list reports only when the indices in view
    // change, so when the rows change under the same indices, the row now at that
    // index is the one in view, and an id kept from the old rows would be wrong.
    const topRowIndex = useRef<number | undefined>(undefined)
    const onTopRowIndex = useCallback((index: number) => {
        topRowIndex.current = index
    }, [])
    // How far a run's first page loads. A refetch of a run keeps its first page as
    // long as it was, up to its own last group row. A new run whose only change is a
    // fold (a group, Expand all, Collapse all) loads as many rows as the reader
    // had (counted as a page's limit counts them), and up to the group row at or above the row at the top of the view, so the
    // rows in view stay through the fold even when they were closed groups with no
    // detail rows (F11). That group row must be in the shown pages, so the fold never
    // loads past what was loaded; with none, the row count alone decides. Any other
    // new run starts with one page.
    const firstPagePlace = (runBody: RunBody, queryKey: QueryKey): ReaderPlace => {
        const first = queryClient.getQueryData<InfiniteData<RunPage>>(queryKey)?.pages[0]
        if (first) {
            const results = resultsOf([first])
            return { rows: pageRowsOf(results), groupRow: lastGroupRowOf(results) }
        }
        const before = shown.current
        if (before?.foldKey !== foldKeyOf(runBody)) return NO_PLACE
        // The list draws the merged rows, so the reported index counts in them.
        const merged = mergePages(before.results)
        return {
            rows: pageRowsOf(before.results),
            groupRow: groupRowAt([merged], rowIdAt(merged, topRowIndex.current)),
        }
    }
    const queryKey = [...RUN_QUERY_KEY, body]
    // The key of the run in view, read by a loadMore or rerun that waits across renders.
    const runKey = useRef<QueryKey>(queryKey)
    // Set at once by loadMore, so a second call while it waits starts nothing.
    const waitingForMore = useRef(false)
    // Shows the loading state while a next page waits for the fetch in flight.
    const [isWaitingForMore, setIsWaitingForMore] = useState(false)
    const query = useInfiniteQuery({
        queryKey,
        enabled: body !== null,
        initialPageParam: '',
        queryFn: async ({ pageParam, signal, queryKey }): Promise<RunPage> => {
            if (!body) throw new Error('reports.run: a run needs a body')
            const place = pageParam === '' ? firstPagePlace(body, queryKey) : NO_PLACE
            const outcome =
                pageParam === ''
                    ? await runFirstPage(body, place.rows, signal, place.groupRow)
                    : await runReport(withPage(body, pageParam), signal)
            // A later page's refusal is thrown, so the query keeps the pages shown and
            // marks only the next page failed (R8).
            if (outcome.kind !== 'ok' && pageParam !== '') throw new LoadMoreRefused(outcome)
            if (outcome.kind === 'ok' && pageParam === '' && 'reportId' in body) {
                useReportRecentStore
                    .getState()
                    .markRun(userId, body.reportId, new Date().toISOString())
            }
            return { ...outcome, rowsKey: rowsKeyOf(body), foldKey: foldKeyOf(body) }
        },
        getNextPageParam: last =>
            last.kind === 'ok' ? last.result.page?.next || undefined : undefined,
        placeholderData: keepPreviousData,
        retry: (count, error) => count < 2 && isTransientRunError(error),
        refetchOnWindowFocus: false,
    })
    const pages = query.data?.pages ?? []
    shown.current = pages[0] ? { foldKey: pages[0].foldKey, results: resultsOf(pages) } : null
    // A refetch and a next page cancel each other's fetch in flight. A live rerun
    // that cancelled a next page would leave the list a page short for good: the list
    // asks for more only when its rows change, and the rerun's rows are the same. A
    // next page that cancelled a rerun would drop the rerun's figures. So each waits
    // until no fetch of the run is in flight (cancelRefetch: false joins one) and then
    // starts its own with no await between the last check and the start. A next page
    // and a rerun can wait on the same fetch (a cached run's mount refetch, a reconnect
    // refetch); the one that wakes first starts its fetch, and the other sees it and
    // waits again. The query client is read, not the last render, because a render
    // lags a fetch that just started.
    runKey.current = queryKey
    // The run whose last next page failed (R8), by its query key's text. Set when a
    // next page fails, cleared when one succeeds; it counts only while the query is
    // in error, so a rerun that lands clears it too.
    const runKeyText = JSON.stringify(queryKey)
    const failedNextPageOf = useRef<string | null>(null)
    const isLoadMoreError = query.isError && failedNextPageOf.current === runKeyText
    const { topics, ...view } = runView({
        hasBody: body !== null,
        pages,
        isPending: query.isPending,
        isError: query.isError,
        isLoadMoreError,
        isPlaceholderData: query.isPlaceholderData,
    })
    const isRunFetching = () => {
        const state = queryClient.getQueryState(runKey.current)
        return state !== undefined && state.fetchStatus !== 'idle'
    }
    const whenIdle = async (start: () => Promise<unknown>) => {
        while (isRunFetching()) await query.refetch({ cancelRefetch: false })
        await start()
    }
    useLiveTopics(topics, () => whenIdle(() => query.refetch()))
    const startNextPage = () => {
        if (!query.hasNextPage || query.isFetchingNextPage || waitingForMore.current) return
        waitingForMore.current = true
        setIsWaitingForMore(true)
        const key = runKeyText
        void whenIdle(async () => {
            const next = await query.fetchNextPage()
            failedNextPageOf.current = next.isFetchNextPageError ? key : null
        }).finally(() => {
            // Also redraws, so the mark set above is read.
            waitingForMore.current = false
            setIsWaitingForMore(false)
        })
    }
    // A failed next page waits for Retry: the list asks for more on every redraw,
    // and the same request would fail again.
    const loadMore = () => {
        if (isLoadMoreError) return
        startNextPage()
    }
    const isLoadingMore = query.isFetchingNextPage || isWaitingForMore
    return {
        ...view,
        shownRowsKey: pages[0]?.rowsKey,
        onTopRowIndex,
        hasMore: query.hasNextPage,
        isLoadingMore,
        loadMore,
        loadMoreFailed: isLoadMoreError && !isLoadingMore,
        retryLoadMore: startNextPage,
        retry: () => void query.refetch(),
    }
}
