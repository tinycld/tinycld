import { keepPreviousData, useInfiniteQuery } from '@tanstack/react-query'
import {
    isTransientRunError,
    RUN_QUERY_KEY,
    type RunBody,
    type RunPage,
    rowsKeyOf,
    runReport,
} from '../lib/run-client'
import { type RunView, runView } from '../lib/run-state'
import { useReportRecentStore } from '../stores/report-recent-store'
import { useLiveTopics } from './useLiveTopics'

export const RUN_PAGE_SIZE = 200

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

function withPage(body: RunBody, cursor: string): RunBody {
    return {
        ...body,
        options: { ...body.options, limit: RUN_PAGE_SIZE, cursor: cursor || undefined },
    }
}

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
    const query = useInfiniteQuery({
        queryKey: [...RUN_QUERY_KEY, body],
        enabled: body !== null,
        initialPageParam: '',
        queryFn: async ({ pageParam, signal }): Promise<RunPage> => {
            if (!body) throw new Error('reports.run: a run needs a body')
            const outcome = await runReport(withPage(body, pageParam), signal)
            if (outcome.kind === 'ok' && pageParam === '' && 'reportId' in body) {
                useReportRecentStore
                    .getState()
                    .markRun(userId, body.reportId, new Date().toISOString())
            }
            return { ...outcome, rowsKey: rowsKeyOf(body) }
        },
        getNextPageParam: last =>
            last.kind === 'ok' ? last.result.page?.next || undefined : undefined,
        placeholderData: keepPreviousData,
        retry: (count, error) => count < 2 && isTransientRunError(error),
        refetchOnWindowFocus: false,
    })
    const { topics, ...view } = runView({
        hasBody: body !== null,
        pages: query.data?.pages ?? [],
        isPending: query.isPending,
        isError: query.isError,
        isPlaceholderData: query.isPlaceholderData,
    })
    useLiveTopics(topics, () => query.refetch().then(() => undefined))
    return {
        ...view,
        shownRowsKey: query.data?.pages[0]?.rowsKey,
        hasMore: query.hasNextPage,
        isLoadingMore: query.isFetchingNextPage,
        loadMore: () => {
            if (query.hasNextPage && !query.isFetchingNextPage) void query.fetchNextPage()
        },
        retry: () => void query.refetch(),
    }
}
