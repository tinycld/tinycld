import { describe, expect, it } from 'vitest'
import { shouldDismissSwipe } from '../../ui/swipe-dismiss/should-dismiss'

// The drag-to-dismiss decision is the one piece of non-RN logic in
// useSwipeToDismiss worth pinning: a slow short drag should snap back, while a
// long drag OR a fast flick should dismiss. Mirrors the thresholds the pan
// gesture inlines.
//
// `dir` is the sign of the off-screen direction, so every case is asserted in
// BOTH orientations — a helper shared by right/bottom panels (+1) and left/top
// ones (-1) is only correct if the sign genuinely mirrors it.
const AWAY = 1 // a 'right' or 'bottom' panel: it leaves in the positive direction
const TOWARD = -1 // a 'left' or 'top' panel: it leaves in the negative direction

describe('shouldDismissSwipe', () => {
    it('keeps the panel open for a short, slow drag', () => {
        expect(shouldDismissSwipe(40, 100, AWAY)).toBe(false)
        expect(shouldDismissSwipe(-40, -100, TOWARD)).toBe(false)
    })

    it('treats the thresholds as exclusive, so exactly at them does not dismiss', () => {
        expect(shouldDismissSwipe(100, 500, AWAY)).toBe(false)
        expect(shouldDismissSwipe(-100, -500, TOWARD)).toBe(false)
    })

    it('dismisses when dragged past the distance threshold', () => {
        expect(shouldDismissSwipe(101, 0, AWAY)).toBe(true)
        expect(shouldDismissSwipe(300, 0, AWAY)).toBe(true)
        expect(shouldDismissSwipe(-101, 0, TOWARD)).toBe(true)
        expect(shouldDismissSwipe(-300, 0, TOWARD)).toBe(true)
    })

    it('dismisses on a fast flick even with little travel', () => {
        expect(shouldDismissSwipe(10, 501, AWAY)).toBe(true)
        expect(shouldDismissSwipe(0, 900, AWAY)).toBe(true)
        expect(shouldDismissSwipe(-10, -501, TOWARD)).toBe(true)
        expect(shouldDismissSwipe(0, -900, TOWARD)).toBe(true)
    })

    // The case that matters for a two-directional helper: dragging a panel
    // back INTO the edge it rests on must never dismiss it, however hard it is
    // flicked. Without the sign this reads as a large positive travel and
    // closes the panel the user was pushing shut.
    it('never dismisses on a drag or flick toward the edge', () => {
        expect(shouldDismissSwipe(-300, -800, AWAY)).toBe(false)
        expect(shouldDismissSwipe(-20, -800, AWAY)).toBe(false)
        expect(shouldDismissSwipe(300, 800, TOWARD)).toBe(false)
        expect(shouldDismissSwipe(20, 800, TOWARD)).toBe(false)
    })
})
