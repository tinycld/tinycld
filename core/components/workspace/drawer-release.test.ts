import { describe, expect, it } from 'vitest'
import { shouldSettleDrawerOpen } from './drawer-release'

const PANEL = 280
const PAST_HALF = -97
const BEFORE_HALF = -200

describe('shouldSettleDrawerOpen', () => {
    it('opens on a fast rightward fling, even before half', () => {
        expect(shouldSettleDrawerOpen(BEFORE_HALF, 80, 1200, PANEL)).toBe(true)
    })

    it('closes on a fast leftward fling, even past half', () => {
        expect(shouldSettleDrawerOpen(PAST_HALF, -60, -1200, PANEL)).toBe(false)
    })

    it('decides a slow drag by position', () => {
        expect(shouldSettleDrawerOpen(PAST_HALF, 183, 100, PANEL)).toBe(true)
        expect(shouldSettleDrawerOpen(BEFORE_HALF, 80, 100, PANEL)).toBe(false)
        expect(shouldSettleDrawerOpen(PAST_HALF, -97, -100, PANEL)).toBe(true)
        expect(shouldSettleDrawerOpen(BEFORE_HALF, -200, -100, PANEL)).toBe(false)
    })

    // Unevenly stamped input events can report a fling against the drag
    // (seen: a rightward drag at -11,000 px/s); position must decide then.
    it('ignores a fling opposite to the net translation', () => {
        expect(shouldSettleDrawerOpen(PAST_HALF, 183, -11000, PANEL)).toBe(true)
        expect(shouldSettleDrawerOpen(BEFORE_HALF, 80, -11000, PANEL)).toBe(false)
        expect(shouldSettleDrawerOpen(BEFORE_HALF, -200, 11000, PANEL)).toBe(false)
        expect(shouldSettleDrawerOpen(PAST_HALF, -97, 11000, PANEL)).toBe(true)
    })
})
