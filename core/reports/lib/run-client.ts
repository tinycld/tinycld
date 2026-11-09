import type { QueryClient } from '@tanstack/react-query'
import { isRecord } from '@tinycld/core/lib/errors'
import { pb } from '@tinycld/core/lib/pocketbase'
import { ClientResponseError } from 'pocketbase'
import { mergePages } from './rows'
import type { ReportSpec } from './spec'
import type { CatalogSource, ParamOptionsPage, Problem, ResultRow, RunResult } from './types'

// The report endpoints (core/server/reports/endpoint.go, catalog.go, options.go). A
// run is a read sent by POST, because a draft spec does not fit in a URL. The
// refusals a person can cause (400, 403, 408, 422) come back as data, so the global
// query error handler does not report them; every other failure is thrown.

export const RUN_PATH = '/api/reports/run'
export const SOURCES_PATH = '/api/reports/sources'
export const PARAM_OPTIONS_PATH = '/api/reports/param-options'

/** One key of a reader's sort: a column of the result, ascending or descending. */
export type ViewSort = { col: string; dir: 'asc' | 'desc' }

export interface RunOptions {
    expand?: unknown[][]
    expandAll?: boolean
    /** Groups the reader closed under an open level or Expand all (J1b); not in the rows or fold key. */
    collapse?: unknown[][]
    cursor?: string
    limit?: number
    /** The reader's sort, before the report's own; detail rows only, inside their groups. */
    sort?: ViewSort[]
    /** The reader's day, which presets resolve against. */
    today: string
}

export type RunBody =
    | { reportId: string; params: Record<string, unknown>; options: RunOptions }
    | { spec: ReportSpec; params: Record<string, unknown>; options: RunOptions }

export type RunRefusal =
    | { kind: 'invalid'; message: string; problems: Problem[] }
    | { kind: 'missing'; message: string; problems: Problem[] }
    | { kind: 'no-access' }
    | { kind: 'timeout' }

export type RunOutcome = { kind: 'ok'; result: RunResult } | RunRefusal

/** One page of a run, tagged with the rows key and fold key of the body that asked for it. */
export type RunPage = RunOutcome & { rowsKey: string; foldKey: string }

// What decides a run's rows, apart from its params (the server resolves those and
// returns them on the result): the saved report, or the draft spec without its
// display, which holds only line layouts per width, and the reader's sort. Other
// options are left out, so a group fold, the next page or a new day keeps the
// reader's place; a new sort is a new order, so its list starts at the top (F11).
// Without a sort the key is as it always was (R11). The param definitions
// (spec.params) stay in the key on purpose: a param-def edit is a design edit.
export function rowsKeyOf(body: RunBody): string {
    const sort = body.options.sort?.length ? `|sort:${JSON.stringify(body.options.sort)}` : ''
    if ('reportId' in body) return `report:${body.reportId}${sort}`
    const { display: _display, ...rowsSpec } = body.spec
    return `spec:${JSON.stringify(rowsSpec)}${sort}`
}

// Two bodies with the same fold key ask for the same run but for which groups it
// opens (and which page): a fold keeps the reader's place (F11).
export function foldKeyOf(body: RunBody): string {
    const {
        expand: _expand,
        expandAll: _expandAll,
        collapse: _collapse,
        cursor: _cursor,
        limit: _limit,
        ...options
    } = body.options
    return JSON.stringify({ ...body, options })
}

// What a page's limit counts (J1a, R3): group, opening, detail and subtotal rows.
// Total and computed rows repeat on every page and never count.
const COUNTED_KINDS: ReadonlySet<ResultRow['kind']> = new Set([
    'group',
    'opening',
    'detail',
    'subtotal',
])

/** How many rows of a run's result a page's limit counts. */
export function pageRowCount(result: RunResult): number {
    return result.rows.filter(row => COUNTED_KINDS.has(row.kind)).length
}

export function problemsOf(data: unknown): Problem[] {
    if (!isRecord(data)) return []
    const out: Problem[] = []
    for (const [path, problem] of Object.entries(data)) {
        if (
            isRecord(problem) &&
            typeof problem.code === 'string' &&
            typeof problem.message === 'string'
        ) {
            out.push({ path, code: problem.code, message: problem.message })
        }
    }
    return out.sort((a, b) => a.path.localeCompare(b.path))
}

export function refusalOf(error: unknown): RunRefusal | null {
    if (!(error instanceof ClientResponseError)) return null
    const body = isRecord(error.response) ? error.response : {}
    const message = typeof body.message === 'string' ? body.message : ''
    switch (error.status) {
        case 400:
            return { kind: 'invalid', message, problems: problemsOf(body.data) }
        case 422:
            return { kind: 'missing', message, problems: problemsOf(body.data) }
        case 403:
            return { kind: 'no-access' }
        case 408:
            return { kind: 'timeout' }
    }
    return null
}

/** The server could not be reached, or failed (5xx, including the read-only 503). */
export function isTransientRunError(error: unknown): boolean {
    return error instanceof ClientResponseError && (error.status === 0 || error.status >= 500)
}

export async function runReport(body: RunBody, signal?: AbortSignal): Promise<RunOutcome> {
    try {
        // requestKey null: two reports may run at once (a frozen tab, the builder's
        // preview); the SDK would otherwise cancel one for the other.
        const result = await pb.send<RunResult>(RUN_PATH, {
            method: 'POST',
            body,
            signal,
            requestKey: null,
        })
        return { kind: 'ok', result }
    } catch (error) {
        const refusal = refusalOf(error)
        if (refusal) return refusal
        throw error
    }
}

/** The rows a run page asks for (group, opening, detail and subtotal rows count, J1a). */
export const RUN_PAGE_SIZE = 200
/** The most rows the server gives a page (run.go runLimits.maxPage). */
export const RUN_MAX_PAGE = 1000

export function withPage(body: RunBody, cursor: string, limit = RUN_PAGE_SIZE): RunBody {
    return {
        ...body,
        options: { ...body.options, limit, cursor: cursor || undefined },
    }
}

/** The id of the last group row in these results, in document order. */
export function lastGroupRowOf(results: readonly RunResult[]): string | undefined {
    for (let p = results.length - 1; p >= 0; p--) {
        const rows = results[p].rows
        for (let r = rows.length - 1; r >= 0; r--) {
            if (rows[r].kind === 'group') return rows[r].id
        }
    }
    return undefined
}

/**
 * The id of the group row at or above the row `rowId` in these results, in document
 * order: the group the reader is in when that row is at the top of the view.
 * Undefined when the results do not hold the row, or no group row comes before it.
 */
export function groupRowAt(
    results: readonly RunResult[],
    rowId: string | undefined
): string | undefined {
    if (rowId === undefined) return undefined
    let group: string | undefined
    for (const result of results) {
        for (const row of result.rows) {
            if (row.kind === 'group') group = row.id
            if (row.id === rowId) return group
        }
    }
    return undefined
}

/**
 * A run's first page, holding at least `target` rows (what a page's limit counts,
 * `pageRowCount`) and the row `untilRow` when the run has them: the server's pages are fetched in turn and joined into one
 * (mergePages), and the next page follows from the last one's cursor. A target of 0
 * with no row is one page. The first request asks for the usual page or the target;
 * when a page has come and only the row is still missing, each next request asks
 * for the server's most, so a row far down takes few requests.
 */
export async function runFirstPage(
    body: RunBody,
    target: number,
    signal?: AbortSignal,
    untilRow?: string
): Promise<RunOutcome> {
    const results: RunResult[] = []
    let cursor = ''
    let loaded = 0
    let holdsRow = !untilRow
    do {
        const limit =
            results.length > 0 && loaded >= target && !holdsRow
                ? RUN_MAX_PAGE
                : Math.min(RUN_MAX_PAGE, Math.max(RUN_PAGE_SIZE, target - loaded))
        const outcome = await runReport(withPage(body, cursor, limit), signal)
        if (outcome.kind !== 'ok') return outcome
        results.push(outcome.result)
        loaded += pageRowCount(outcome.result)
        holdsRow ||= outcome.result.rows.some(row => row.id === untilRow)
        cursor = outcome.result.page?.next ?? ''
    } while (cursor && (loaded < target || !holdsRow))
    return { kind: 'ok', result: mergePages(results) }
}

/**
 * One param-options request (options.go paramOptionsBody): a saved report the reader
 * can view or a draft spec, the param, and what to read. `group` reads one group's
 * options ('' is the blank group); `values` asks for the labels of those values;
 * `params` are the run's other params, which a `where` may read.
 */
export type ParamOptionsBody = ({ reportId: string } | { spec: ReportSpec }) & {
    param: string
    search?: string
    after?: string
    group?: string
    values?: string[]
    limit?: number
    params?: Record<string, unknown>
    /** The reader's day, which presets resolve against. */
    today: string
}

/** One page of a choice param's options, read under the reader's rules. */
export async function fetchParamOptions(
    body: ParamOptionsBody,
    signal?: AbortSignal
): Promise<ParamOptionsPage> {
    // requestKey null: a picker's searches overlap; each one's signal cancels it.
    return pb.send<ParamOptionsPage>(PARAM_OPTIONS_PATH, {
        method: 'POST',
        body,
        signal,
        requestKey: null,
    })
}

export async function fetchCatalog(signal?: AbortSignal): Promise<CatalogSource[]> {
    const res = await pb.send<{ sources: CatalogSource[] }>(SOURCES_PATH, {
        method: 'GET',
        signal,
        requestKey: null,
    })
    return res.sources
}

/** The react-query key every report run sits under; the run body follows it. */
export const RUN_QUERY_KEY = ['reports', 'run'] as const

function isRunOf(queryKey: readonly unknown[], reportId: string): boolean {
    const body = queryKey[RUN_QUERY_KEY.length]
    return (
        typeof body === 'object' &&
        body !== null &&
        'reportId' in body &&
        body.reportId === reportId
    )
}

/**
 * Drops every cached run of a saved report. After its spec changes, a cached run
 * would show the old figures as current until the next run answered.
 */
export function forgetSavedRuns(queryClient: QueryClient, reportId: string) {
    queryClient.removeQueries({
        queryKey: RUN_QUERY_KEY,
        predicate: query => isRunOf(query.queryKey, reportId),
    })
}

const SORT_OPTION_PATH = 'options.sort'

/**
 * Why the run refused the reader's sort (a column the report no longer has, R8):
 * the message of the first problem under options.sort, else null.
 */
export function refusalSortProblem(problems: readonly Problem[]): string | null {
    const problem = problems.find(
        p => p.path === SORT_OPTION_PATH || p.path.startsWith(`${SORT_OPTION_PATH}.`)
    )
    return problem?.message ?? null
}
