import { describe, expect, it } from 'vitest'
import {
    createReachabilityTracker,
    FAILURE_THRESHOLD,
    isServerDownFailure,
    ROLLING_WINDOW_MS,
} from '../server-reachability'

// A network-level failure as PocketBase surfaces it: a ClientResponseError
// with status 0 (the request left but never got an HTTP response).
const netFail = { status: 0, isAbort: false }
const reqPath = '/api/collections/messages/records'

describe('isServerDownFailure', () => {
    it('treats a no-status transport error as server-down', () => {
        expect(isServerDownFailure(netFail)).toBe(true)
    })

    it('ignores aborts flagged by PocketBase', () => {
        expect(isServerDownFailure({ status: 0, isAbort: true })).toBe(false)
    })

    it('ignores raw AbortError / DOMException shapes that bypass isAbort', () => {
        expect(isServerDownFailure({ name: 'AbortError' })).toBe(false)
        expect(isServerDownFailure({ message: 'Aborted' })).toBe(false)
    })

    it('ignores any positive HTTP status (app-level, server answered)', () => {
        expect(isServerDownFailure({ status: 500 })).toBe(false)
        expect(isServerDownFailure({ status: 401 })).toBe(false)
    })

    it('ignores non-object errors', () => {
        expect(isServerDownFailure(null)).toBe(false)
        expect(isServerDownFailure('boom')).toBe(false)
        expect(isServerDownFailure(undefined)).toBe(false)
    })
})

describe('reachability tracker — sustained-failure flip', () => {
    it('does NOT flip on a single network failure (the reported over-trigger)', () => {
        const t = createReachabilityTracker()
        // A single blip, even long after boot, must not show the badge.
        expect(t.record(reqPath, false, netFail, 60_000)).toBe(null)
    })

    it('flips after FAILURE_THRESHOLD failures within the window', () => {
        const t = createReachabilityTracker()
        expect(t.record(reqPath, false, netFail, 1_000)).toBe(null)
        // Threshold is 2 by default — second failure inside the window flips.
        expect(FAILURE_THRESHOLD).toBe(2)
        expect(t.record(reqPath, false, netFail, 1_500)).toBe('down')
    })

    it('does not flip when failures are spread beyond the rolling window', () => {
        const t = createReachabilityTracker()
        expect(t.record(reqPath, false, netFail, 0)).toBe(null)
        // Second failure lands after the window — streak resets, no flip.
        expect(t.record(reqPath, false, netFail, ROLLING_WINDOW_MS + 1)).toBe(null)
    })

    it('a success between failures resets the streak', () => {
        const t = createReachabilityTracker()
        t.record(reqPath, false, netFail, 1_000)
        t.record(reqPath, true, null, 1_200) // recover the streak
        // Next single failure should again be tolerated, not flip.
        expect(t.record(reqPath, false, netFail, 1_400)).toBe(null)
    })
})

describe('reachability tracker — recovery signal', () => {
    it("signals 'up' on the success that ends a failing streak", () => {
        const t = createReachabilityTracker()
        t.record(reqPath, false, netFail, 1_000)
        expect(t.record(reqPath, true, null, 1_100)).toBe('up')
    })

    it("does not churn 'up' on successes when nothing was failing", () => {
        const t = createReachabilityTracker()
        expect(t.record(reqPath, true, null, 1_000)).toBe(null)
        expect(t.record(reqPath, true, null, 2_000)).toBe(null)
    })
})

describe('reachability tracker — excluded failure shapes', () => {
    it('never counts auth-refresh failures toward the streak', () => {
        const t = createReachabilityTracker()
        const refresh = '/api/collections/users/auth-refresh'
        // Many failed refreshes must not flip the badge — they drive re-auth.
        expect(t.record(refresh, false, netFail, 1_000)).toBe(null)
        expect(t.record(refresh, false, netFail, 1_500)).toBe(null)
        expect(t.record(refresh, false, netFail, 2_000)).toBe(null)
    })

    it('never counts aborts toward the streak', () => {
        const t = createReachabilityTracker()
        const abort = { status: 0, isAbort: true }
        expect(t.record(reqPath, false, abort, 1_000)).toBe(null)
        expect(t.record(reqPath, false, abort, 1_500)).toBe(null)
    })

    it('never counts HTTP error responses toward the streak', () => {
        const t = createReachabilityTracker()
        expect(t.record(reqPath, false, { status: 500 }, 1_000)).toBe(null)
        expect(t.record(reqPath, false, { status: 503 }, 1_500)).toBe(null)
    })

    it('does not let an excluded failure block a real flip', () => {
        const t = createReachabilityTracker()
        // An abort then two genuine failures still flips on the real ones.
        t.record(reqPath, false, { status: 0, isAbort: true }, 1_000)
        expect(t.record(reqPath, false, netFail, 1_100)).toBe(null)
        expect(t.record(reqPath, false, netFail, 1_200)).toBe('down')
    })
})
