import { describe, expect, it } from 'vitest'
import { isNearEnd, NEAR_END_PX } from '../near-end'

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

    it('is true for a page that does not fill the view, which never scrolls', () => {
        expect(
            isNearEnd({
                contentOffset: { y: 0 },
                layoutMeasurement: { height: 400 },
                contentSize: { height: 300 },
            })
        ).toBe(true)
    })
})
