import {
    type InfiniteData,
    keepPreviousData,
    type QueryKey,
    useInfiniteQuery,
    useQueryClient,
} from '@tanstack/react-query'
import { useRef } from 'react'
import {
    detailRowCount,
    foldKeyOf,
    isTransientRunError,
    lastGroupRowOf,
    RUN_QUERY_KEY,
    type RunBody,
    type RunPage,
    rowsKeyOf,
    runFirstPage,
    runReport,
    withPage,
} from '../lib/run-client'
import { type RunView, runView } from '../lib/run-state'
import { useReportRecentStore } from '../stores/report-recent-store'
import { useLiveTopics } from './useLiveTopics'

export interface ReportRun extends Omit<RunView, 'topics'> {
    /**
     * The rows key of the body whose pages are shown. While a changed run loads, the
     * previous body's pages stay in view, so this changes when the new run lands.
     * It is optional because a hand-built ReportRun (a mockup) has no run body.
     * Without it, a design change does not reset the list to the top.
     */
    shownRowsKey?: string
    hasMore: boolean
    isLoadingMore: boolean
    loadMore(): void
    retry(): void
}

const resultsOf = (pages: readonly RunPage[]) =>
    pages.flatMap(page => (page.kind === 'ok' ? [page.result] : []))

/**
 * How far a run's first page loads: its detail rows, and the last group row it must
 * hold (with every group closed, the reader's place is a group row, not a detail row).
 */
type ReaderPlace = { detailRows: number; groupRow?: string }

const placeOf = (pages: readonly RunPage[]): ReaderPlace => {
    const results = resultsOf(pages)
    return {
        detailRows: results.reduce((n, result) => n + detailRowCount(result), 0),
        groupRow: lastGroupRowOf(results),
    }
}

/** The pages in view: the body they answer (its fold key) and the reader's place. */
type ShownPages = { foldKey: string; place: ReaderPlace }

const NO_PLACE: ReaderPlace = { detailRows: 0 }

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
    // How far a run's first page loads. A refetch of a run keeps its first page as
    // long as it was. A new run whose only change is a fold (a group, Expand all,
    // Collapse all) loads as far as the reader had: as many detail rows, and up to the
    // last group row loaded, so the rows in view stay through the fold even when they
    // were closed groups with no detail rows (F11). Any other new run starts with one
    // page.
    const firstPagePlace = (runBody: RunBody, queryKey: QueryKey): ReaderPlace => {
        const first = queryClient.getQueryData<InfiniteData<RunPage>>(queryKey)?.pages[0]
        if (first) return placeOf([first])
        const before = shown.current
        return before?.foldKey === foldKeyOf(runBody) ? before.place : NO_PLACE
    }
    const query = useInfiniteQuery({
        queryKey: [...RUN_QUERY_KEY, body],
        enabled: body !== null,
        initialPageParam: '',
        queryFn: async ({ pageParam, signal, queryKey }): Promise<RunPage> => {
            if (!body) throw new Error('reports.run: a run needs a body')
            const place = pageParam === '' ? firstPagePlace(body, queryKey) : NO_PLACE
            const outcome =
                pageParam === ''
                    ? await runFirstPage(body, place.detailRows, signal, place.groupRow)
                    : await runReport(withPage(body, pageParam), signal)
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
    shown.current = pages[0] ? { foldKey: pages[0].foldKey, place: placeOf(pages) } : null
    const { topics, ...view } = runView({
        hasBody: body !== null,
        pages,
        isPending: query.isPending,
        isError: query.isError,
        isPlaceholderData: query.isPlaceholderData,
    })
    useLiveTopics(topics, () => query.refetch().then(() => undefined))
    return {
        ...view,
        shownRowsKey: pages[0]?.rowsKey,
        hasMore: query.hasNextPage,
        isLoadingMore: query.isFetchingNextPage,
        loadMore: () => {
            if (query.hasNextPage && !query.isFetchingNextPage) void query.fetchNextPage()
        },
        retry: () => void query.refetch(),
    }
}
