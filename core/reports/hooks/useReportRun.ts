import { keepPreviousData, useInfiniteQuery } from '@tanstack/react-query'
import { mergePages } from '../lib/rows'
import {
    isTransientRunError,
    RUN_QUERY_KEY,
    type RunBody,
    type RunOutcome,
    type RunRefusal,
    runReport,
} from '../lib/run-client'
import type { Problem, RunResult } from '../lib/types'
import { useReportRecentStore } from '../stores/report-recent-store'
import { useLiveTopics } from './useLiveTopics'

export const RUN_PAGE_SIZE = 200
const NO_TOPICS: string[] = []
const NO_PROBLEMS: Problem[] = []

export type RunState =
    | 'idle'
    | 'loading'
    | 'error'
    | 'invalid'
    | 'missing'
    | 'no-access'
    | 'timeout'
    | 'ready'

export interface ReportRun {
    state: RunState
    result: RunResult | undefined
    problems: Problem[]
    /** A changed run is loading behind the figures shown (R29). */
    isUpdating: boolean
    hasMore: boolean
    isLoadingMore: boolean
    loadMore(): void
    retry(): void
}

function withPage(body: RunBody, cursor: string): RunBody {
    return {
        ...body,
        options: { ...body.options, limit: RUN_PAGE_SIZE, cursor: cursor || undefined },
    }
}

function firstRefusal(pages: readonly RunOutcome[]): RunRefusal | undefined {
    for (const page of pages) if (page.kind !== 'ok') return page
    return undefined
}

function runState(
    hasBody: boolean,
    isPending: boolean,
    isError: boolean,
    refusal: RunRefusal | undefined
): RunState {
    if (!hasBody) return 'idle'
    if (refusal) return refusal.kind
    if (isError) return 'error'
    return isPending ? 'loading' : 'ready'
}

// Runs a report through POST /api/reports/run (R9). A report without groups pages
// by keyset; a grouped one answers in one page. The result's topics keep it live.
export function useReportRun({
    body,
    userId,
}: {
    body: RunBody | null
    userId: string
}): ReportRun {
    const query = useInfiniteQuery({
        queryKey: [...RUN_QUERY_KEY, body],
        enabled: body !== null,
        initialPageParam: '',
        queryFn: async ({ pageParam, signal }) => {
            if (!body) throw new Error('reports.run: a run needs a body')
            const outcome = await runReport(withPage(body, pageParam), signal)
            if (outcome.kind === 'ok' && pageParam === '' && 'reportId' in body) {
                useReportRecentStore
                    .getState()
                    .markRun(userId, body.reportId, new Date().toISOString())
            }
            return outcome
        },
        getNextPageParam: last =>
            last.kind === 'ok' ? last.result.page?.next || undefined : undefined,
        placeholderData: keepPreviousData,
        retry: (count, error) => count < 2 && isTransientRunError(error),
        refetchOnWindowFocus: false,
    })
    const pages = query.data?.pages ?? []
    const refusal = firstRefusal(pages)
    const results = pages.flatMap(page => (page.kind === 'ok' ? [page.result] : []))
    const result = results.length > 0 ? mergePages(results) : undefined
    useLiveTopics(result?.topics ?? NO_TOPICS, () => query.refetch().then(() => undefined))
    return {
        state: runState(body !== null, query.isPending, query.isError, refusal),
        result: refusal ? undefined : result,
        problems: refusal && 'problems' in refusal ? refusal.problems : NO_PROBLEMS,
        isUpdating: query.isPlaceholderData,
        hasMore: query.hasNextPage,
        isLoadingMore: query.isFetchingNextPage,
        loadMore: () => {
            if (query.hasNextPage && !query.isFetchingNextPage) void query.fetchNextPage()
        },
        retry: () => void query.refetch(),
    }
}
