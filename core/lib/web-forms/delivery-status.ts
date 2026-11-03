export type RunLike = { rule: string; fired_at: string; error: string; results: unknown }

export function runSucceeded(run: RunLike): boolean {
    if (run.error) return false
    const results = Array.isArray(run.results) ? (run.results as { status?: string }[]) : []
    return results.every(r => r.status !== 'error')
}

/** A destination's newest run for one submission, or undefined. */
export function newestRun<T extends RunLike>(ruleId: string, runs: T[]): T | undefined {
    let newest: T | undefined
    for (const run of runs) {
        if (run.rule !== ruleId) continue
        if (!newest || run.fired_at > newest.fired_at) newest = run
    }
    return newest
}

/**
 * Delivered when a destination's newest run for the submission succeeded. A
 * destination with no run for it is left out: it was added after the
 * submission, or its old runs were pruned, and neither is a failed delivery.
 *
 * Counting only destinations with a run for the submission is deliberate:
 * counting every destination made before the submission would show a false
 * failure for each submission older than its destination's 200 kept runs.
 * A run the engine never started shows in the rule's run history and the
 * failure notice instead.
 */
export function deliveryStatus(destinationIds: string[], runs: RunLike[]) {
    let delivered = 0
    let total = 0
    for (const id of destinationIds) {
        const newest = newestRun(id, runs)
        if (!newest) continue
        total++
        if (runSucceeded(newest)) delivered++
    }
    return { delivered, total }
}
