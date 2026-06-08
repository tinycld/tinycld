import {
    consumeContextMenuPressSuppression,
    markContextMenuOpenedByLongPress,
} from '@tinycld/core/components/context-menu-press-guard'
import { beforeEach, describe, expect, it } from 'vitest'

// The guard is a module-level singleton (touch gestures are serialized, so one
// stamp at a time is unambiguous). Each test marks a fresh long-press so state
// from a prior test never leaks; the suppression window is well under the gaps
// between the timestamps we use here.
describe('context-menu press guard', () => {
    beforeEach(() => {
        // Consume any leftover suppression far in the future so each test starts
        // from a clean, un-suppressed state.
        consumeContextMenuPressSuppression(Number.MAX_SAFE_INTEGER)
    })

    it('suppresses the press that immediately follows a long-press', () => {
        const t = 1000
        markContextMenuOpenedByLongPress(t)
        // Finger lifts a moment later — this tap must be swallowed.
        expect(consumeContextMenuPressSuppression(t + 50)).toBe(true)
    })

    it('only suppresses one press — the next tap goes through', () => {
        const t = 1000
        markContextMenuOpenedByLongPress(t)
        expect(consumeContextMenuPressSuppression(t + 50)).toBe(true)
        // A second tap in the same window is a real, deliberate tap.
        expect(consumeContextMenuPressSuppression(t + 100)).toBe(false)
    })

    it('does not suppress a tap that arrives after the window closes', () => {
        const t = 1000
        markContextMenuOpenedByLongPress(t)
        // 800ms later is past the ~700ms window: a deliberate later tap.
        expect(consumeContextMenuPressSuppression(t + 800)).toBe(false)
    })

    it('does not suppress when no long-press was marked', () => {
        expect(consumeContextMenuPressSuppression(5000)).toBe(false)
    })
})
