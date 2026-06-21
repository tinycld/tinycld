import { describe, expect, it } from 'vitest'
import { shouldDismissDrawer } from '../../ui/bottom-drawer/should-dismiss'

// The drag-to-dismiss decision is the one piece of non-RN logic in BottomDrawer
// worth pinning: a slow short drag should snap back, while a long drag OR a fast
// flick should dismiss. Mirrors the thresholds the pan gesture uses.
describe('shouldDismissDrawer', () => {
    it('keeps the sheet open for a short, slow drag', () => {
        expect(shouldDismissDrawer(40, 100)).toBe(false)
        expect(shouldDismissDrawer(100, 500)).toBe(false) // exactly at thresholds → not past
    })

    it('dismisses when dragged past the distance threshold', () => {
        expect(shouldDismissDrawer(101, 0)).toBe(true)
        expect(shouldDismissDrawer(300, 0)).toBe(true)
    })

    it('dismisses on a fast downward flick even with little travel', () => {
        expect(shouldDismissDrawer(10, 501)).toBe(true)
        expect(shouldDismissDrawer(0, 900)).toBe(true)
    })

    it('does not dismiss on an upward flick', () => {
        // Negative velocity (flicking up) must never dismiss.
        expect(shouldDismissDrawer(20, -800)).toBe(false)
    })
})
