/**
 * Pure retry/watchdog decisions for LazySidebarBoundary, split out so the
 * recovery rules can be unit-tested without rendering React (the boundary's
 * real failure modes — a Metro chunk race — only reproduce under load).
 */

/**
 * The attempt counter after a retry: increments while under the cap, otherwise
 * stays put so a genuinely-broken chunk doesn't loop forever. The counter
 * doubles as the React key on the Suspense subtree, so a change forces a
 * remount (and a fresh lazy `import()`); no change means "give up".
 */
export function nextAttempt(attempt: number, maxRetries: number): number {
    return attempt < maxRetries ? attempt + 1 : attempt
}

/**
 * Whether the watchdog should remount when its timer fires: only if the subtree
 * still hasn't committed (`mounted` false — still in the skeleton) AND we have
 * retries left. A committed sidebar (mounted true) must never be torn down.
 */
export function shouldRetryOnTimeout(
    mounted: boolean,
    attempt: number,
    maxRetries: number
): boolean {
    if (mounted) return false
    return attempt < maxRetries
}
