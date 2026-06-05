// Decides whether a failed `pb.send` request should flip the global
// "server unreachable" signal that drives the offline overlay.
//
// The badge over-triggered because the previous logic flipped on a SINGLE
// network-level failure at any point after a 10s boot window, and counted
// too many error shapes (timeouts, aborts, auth-refresh races) as "the
// server is down". This module fixes both:
//
//   1. A narrower failure classifier (`isServerDownFailure`) that excludes
//      aborts/cancellations and auth-refresh requests — a refresh failing
//      means "re-authenticate", not "the server is unreachable".
//   2. A rolling sustained-failure requirement that applies at ALL times,
//      not just during boot: the signal only flips after N network failures
//      land within a short window. Any success resets the streak.

// Require this many consecutive network failures inside ROLLING_WINDOW_MS
// before declaring the server unreachable.
export const FAILURE_THRESHOLD = 2

// Failures older than this don't count toward the streak. A single blip
// followed by a long-enough quiet gap never trips the badge.
export const ROLLING_WINDOW_MS = 5_000

// PocketBase auth-refresh requests hit `/api/collections/<col>/auth-refresh`.
// A failing refresh should drive re-auth, not the offline badge, so it never
// counts as a server-down signal.
function isAuthRefreshPath(path: string): boolean {
    return path.endsWith('/auth-refresh')
}

// True only for failures that genuinely indicate the server is unreachable:
// a request that left the device but came back with no HTTP status (DNS
// failure, connection refused, TLS error, transport-level timeout).
//
// Explicitly NOT server-down:
//   - aborts / autocancellation (navigation, AbortController) — PocketBase
//     normalizes these into `isAbort`, but we also guard against raw
//     AbortError/DOMException shapes that bypass that normalization.
//   - any response carrying a positive HTTP status (4xx/5xx are app-level;
//     the server clearly answered).
export function isServerDownFailure(err: unknown): boolean {
    if (!err || typeof err !== 'object') return false
    const e = err as {
        status?: number
        isAbort?: boolean
        name?: string
        message?: string
    }
    if (e.isAbort) return false
    if (e.name === 'AbortError' || e.message === 'Aborted') return false
    if (typeof e.status === 'number' && e.status > 0) return false
    return true
}

export interface ReachabilityTracker {
    // Record the outcome of a `pb.send`. Returns 'down' the moment the
    // sustained-failure threshold is crossed, 'up' on a recovering success,
    // and null when the outcome doesn't change the signal.
    record(path: string, ok: boolean, err: unknown, now: number): 'up' | 'down' | null
}

// Factory so the rolling state is injectable and unit-testable with a fake
// clock — no module-level mutable state, no reliance on the real Date.now().
export function createReachabilityTracker(): ReachabilityTracker {
    let failuresInWindow = 0
    let lastFailureAt = 0

    return {
        record(path, ok, err, now) {
            if (ok) {
                const wasFailing = failuresInWindow > 0
                failuresInWindow = 0
                lastFailureAt = 0
                // Only signal recovery if we'd actually been accumulating
                // failures — avoids redundant "up" churn on every success.
                return wasFailing ? 'up' : null
            }

            if (isAuthRefreshPath(path)) return null
            if (!isServerDownFailure(err)) return null

            // Reset the streak if the previous failure aged out of the window:
            // sustained means "close together", not "ever".
            if (now - lastFailureAt > ROLLING_WINDOW_MS) {
                failuresInWindow = 0
            }
            failuresInWindow++
            lastFailureAt = now

            return failuresInWindow >= FAILURE_THRESHOLD ? 'down' : null
        },
    }
}
