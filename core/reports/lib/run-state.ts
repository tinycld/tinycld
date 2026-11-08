import { holdWholeReportRows, mergePages, type ShownResult } from './rows'
import type { RunOutcome, RunRefusal } from './run-client'
import type { Problem, ResultRow, RunResult } from './types'

export type RunState =
    | 'idle'
    | 'loading'
    | 'error'
    | 'invalid'
    | 'missing'
    | 'no-access'
    | 'timeout'
    | 'ready'

export interface RunQueryState {
    hasBody: boolean
    pages: readonly RunOutcome[]
    isPending: boolean
    isError: boolean
    /** The pages are the previous body's answer, shown while this body's run loads. */
    isPlaceholderData: boolean
}

export interface RunView {
    state: RunState
    result: RunResult | undefined
    /**
     * The whole-report rows (Total, computed) held back from `result` while a later
     * page is left (D8). The column measure reads them; the list does not show them.
     */
    heldRows: readonly ResultRow[]
    problems: Problem[]
    /** A changed run is loading behind the figures shown (R29). */
    isUpdating: boolean
    /** The topics of the figures received, which keep the run live. */
    topics: string[]
}

const NO_PROBLEMS: Problem[] = []
const NO_TOPICS: string[] = []
const NO_ROWS: readonly ResultRow[] = []

function firstRefusal(pages: readonly RunOutcome[]): RunRefusal | undefined {
    for (const page of pages) if (page.kind !== 'ok') return page
    return undefined
}

function baseState(query: RunQueryState, refusal: RunRefusal | undefined): RunState {
    if (!query.hasBody) return 'idle'
    if (refusal) return refusal.kind
    if (query.isError) return 'error'
    return query.isPending ? 'loading' : 'ready'
}

// One shown result per pages array (the query keeps the array while its pages stay
// the same), so a render that changes only the run's state keeps the rows: the
// table redraws, and its list re-lays out, only when the rows change.
const mergedByPages = new WeakMap<readonly RunOutcome[], ShownResult | undefined>()

function shownResult(pages: readonly RunOutcome[]): ShownResult | undefined {
    if (mergedByPages.has(pages)) return mergedByPages.get(pages)
    const results = pages.flatMap(page => (page.kind === 'ok' ? [page.result] : []))
    const shown = results.length > 0 ? holdWholeReportRows(mergePages(results)) : undefined
    mergedByPages.set(pages, shown)
    return shown
}

// What a run shows from its query. Placeholder pages are the previous body's answer:
// its figures stay in view while the new run loads, but its refusal does not belong
// to this body, so it shows as loading, not as the old refusal.
export function runView(query: RunQueryState): RunView {
    const refusal = firstRefusal(query.pages)
    const shown = shownResult(query.pages)
    const isStaleRefusal = query.isPlaceholderData && refusal !== undefined
    return {
        state: isStaleRefusal ? 'loading' : baseState(query, refusal),
        result: refusal ? undefined : shown?.result,
        heldRows: refusal ? NO_ROWS : (shown?.held ?? NO_ROWS),
        problems:
            !isStaleRefusal && refusal && 'problems' in refusal ? refusal.problems : NO_PROBLEMS,
        isUpdating: query.isPlaceholderData && !refusal,
        topics: shown?.result.topics ?? NO_TOPICS,
    }
}
