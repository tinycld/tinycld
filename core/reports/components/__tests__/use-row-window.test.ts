import { describe, expect, it } from 'vitest'
import { isNearEnd, NEAR_END_PX, ROW_OVERSCAN, rowWindowBounds } from '../use-row-window'

describe('rowWindowBounds', () => {
    it('mounts the rows in view plus an overscan on each side', () => {
        const bounds = rowWindowBounds({
            firstVisible: 20,
            total: 100,
            rowHeight: 44,
            viewHeight: 440,
        })
        expect(bounds).toEqual({
            first: 20 - ROW_OVERSCAN,
            last: 20 - ROW_OVERSCAN + 10 + ROW_OVERSCAN * 2,
            topSpace: (20 - ROW_OVERSCAN) * 44,
            bottomSpace: (100 - (20 - ROW_OVERSCAN + 10 + ROW_OVERSCAN * 2)) * 44,
        })
    })

    it('never goes empty when the list shrinks under a stale scroll position', () => {
        const bounds = rowWindowBounds({
            firstVisible: 500,
            total: 8,
            rowHeight: 44,
            viewHeight: 440,
        })
        expect(bounds.first).toBe(0)
        expect(bounds.last).toBe(8)
    })
})

describe('isNearEnd', () => {
    it('is true within the near-end distance of the content’s end', () => {
        const metrics = (y: number) => ({
            contentOffset: { y },
            layoutMeasurement: { height: 400 },
            contentSize: { height: 2000 },
        })
        expect(isNearEnd(metrics(2000 - 400 - NEAR_END_PX))).toBe(true)
        expect(isNearEnd(metrics(2000 - 400 - NEAR_END_PX - 1))).toBe(false)
    })

    it('is true for a page that does not fill the scroller, which never scrolls', () => {
        expect(
            isNearEnd({
                contentOffset: { y: 0 },
                layoutMeasurement: { height: 400 },
                contentSize: { height: 300 },
            })
        ).toBe(true)
    })
})
