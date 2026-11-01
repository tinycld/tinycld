import { mergePages } from './rows'
import type { RunOutcome, RunRefusal } from './run-client'
import type { Problem, RunResult } from './types'

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
    problems: Problem[]
    /** A changed run is loading behind the figures shown (R29). */
    isUpdating: boolean
    /** The topics of the figures received, which keep the run live. */
    topics: string[]
}

const NO_PROBLEMS: Problem[] = []
const NO_TOPICS: string[] = []

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

// What a run shows from its query. Placeholder pages are the previous body's answer:
// its figures stay in view while the new run loads, but its refusal does not belong
// to this body, so it shows as loading, not as the old refusal.
export function runView(query: RunQueryState): RunView {
    const refusal = firstRefusal(query.pages)
    const results = query.pages.flatMap(page => (page.kind === 'ok' ? [page.result] : []))
    const merged = results.length > 0 ? mergePages(results) : undefined
    const isStaleRefusal = query.isPlaceholderData && refusal !== undefined
    return {
        state: isStaleRefusal ? 'loading' : baseState(query, refusal),
        result: refusal ? undefined : merged,
        problems:
            !isStaleRefusal && refusal && 'problems' in refusal ? refusal.problems : NO_PROBLEMS,
        isUpdating: query.isPlaceholderData && !refusal,
        topics: merged?.topics ?? NO_TOPICS,
    }
}
