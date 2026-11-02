import { keepPreviousData, useInfiniteQuery, useQueries } from '@tanstack/react-query'
import { todayIso } from '../lib/dates'
import { labelsOfPages } from '../lib/param-options'
import type { ChoiceLabels } from '../lib/params'
import { fetchParamOptions, isTransientRunError } from '../lib/run-client'
import type { ReportSpec } from '../lib/spec'
import type { ParamOptionsPage } from '../lib/types'

/** Where a param's options are read: a saved report the reader can view, or a draft spec. */
export type OptionsSource = { reportId: string } | { spec: ReportSpec }

const OPTIONS_KEY = ['reports', 'param-options'] as const
const LABELS_KEY = ['reports', 'param-labels'] as const
const NO_LABELS: ChoiceLabels = new Map()

const retryTransient = (count: number, error: unknown) => count < 2 && isTransientRunError(error)

/**
 * A choice param's options, a page at a time, searched by label. The server folds
 * case for all of Unicode (tc_fold), so the search is sent as typed and the answer
 * is never filtered again here.
 */
export function useParamOptions(input: {
    source: OptionsSource
    param: string
    search: string
    params: Record<string, unknown>
    isEnabled: boolean
}) {
    const { source, param, search, params, isEnabled } = input
    return useInfiniteQuery({
        queryKey: [...OPTIONS_KEY, source, param, search, params],
        enabled: isEnabled,
        initialPageParam: '',
        queryFn: ({ pageParam, signal }) =>
            fetchParamOptions(
                {
                    ...source,
                    param,
                    search: search || undefined,
                    after: pageParam || undefined,
                    params,
                    today: todayIso(),
                },
                signal
            ),
        getNextPageParam: (last: ParamOptionsPage) => last.next || undefined,
        // While a new search loads, the last answer stays in the list.
        placeholderData: keepPreviousData,
        retry: retryTransient,
    })
}

/** The records and whole groups of one param whose labels a chip or a builder value shows. */
export interface LabelAsk {
    param: string
    values: readonly string[]
    groups: readonly string[]
}

type LabelQuery = { param: string; kind: 'values' | 'groups'; ids: readonly string[] }

function labelQueries(asks: readonly LabelAsk[]): LabelQuery[] {
    return asks.flatMap(ask => [
        ...(ask.values.length > 0
            ? [{ param: ask.param, kind: 'values' as const, ids: ask.values }]
            : []),
        // Groups come only with a first page (F9): one option is enough to read them.
        ...(ask.groups.length > 0
            ? [{ param: ask.param, kind: 'groups' as const, ids: ask.groups }]
            : []),
    ])
}

/**
 * The shown labels of chosen records and whole groups, per param, read under the
 * reader's rules: a record the reader cannot list gets no label. A param's labels
 * are null while they are read (and while there is no source to read them from).
 */
export function useParamOptionLabels(
    source: OptionsSource | null,
    asks: readonly LabelAsk[],
    params: Record<string, unknown>
): ReadonlyMap<string, ChoiceLabels | null> {
    const queries = labelQueries(asks)
    const results = useQueries({
        queries: queries.map(q => ({
            queryKey: [...LABELS_KEY, source, q.param, q.kind, q.ids, params],
            enabled: source !== null,
            retry: retryTransient,
            queryFn: ({ signal }: { signal: AbortSignal }) => {
                if (!source) throw new Error('reports.param-labels: labels need a source')
                const common = { ...source, param: q.param, params, today: todayIso() }
                const body =
                    q.kind === 'values'
                        ? { ...common, values: [...q.ids] }
                        : { ...common, limit: 1 }
                return fetchParamOptions(body, signal)
            },
        })),
    })
    const pagesByParam = new Map<string, ParamOptionsPage[]>()
    const loading = new Set<string>()
    queries.forEach((q, i) => {
        const result = results[i]
        if (!result || result.isPending) loading.add(q.param)
        if (result?.data) {
            pagesByParam.set(q.param, [...(pagesByParam.get(q.param) ?? []), result.data])
        }
    })
    return new Map(
        asks.map(ask => {
            if (loading.has(ask.param)) return [ask.param, null] as const
            const pages = pagesByParam.get(ask.param)
            return [ask.param, pages ? labelsOfPages(pages) : NO_LABELS] as const
        })
    )
}
