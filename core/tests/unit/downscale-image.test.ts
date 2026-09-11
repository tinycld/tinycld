import { fitWithinMaxEdge, MAX_AVATAR_EDGE } from '@tinycld/core/lib/downscale-image'
import { describe, expect, it } from 'vitest'

describe('fitWithinMaxEdge', () => {
    it('leaves an already-small image untouched', () => {
        expect(fitWithinMaxEdge(400, 300)).toEqual({ width: 400, height: 300 })
    })

    it('caps a wide image on its width and keeps the aspect ratio', () => {
        expect(fitWithinMaxEdge(4000, 2000)).toEqual({ width: 1024, height: 512 })
    })

    it('caps a tall image on its height', () => {
        expect(fitWithinMaxEdge(2000, 4000)).toEqual({ width: 512, height: 1024 })
    })

    it('caps a square image on both edges', () => {
        expect(fitWithinMaxEdge(3000, 3000)).toEqual({
            width: MAX_AVATAR_EDGE,
            height: MAX_AVATAR_EDGE,
        })
    })

    it('rounds to whole pixels', () => {
        const { width, height } = fitWithinMaxEdge(3000, 1777)
        expect(Number.isInteger(width)).toBe(true)
        expect(Number.isInteger(height)).toBe(true)
    })

    it('never returns a zero dimension for a degenerate input', () => {
        const { width, height } = fitWithinMaxEdge(10000, 1)
        expect(width).toBeGreaterThan(0)
        expect(height).toBeGreaterThan(0)
    })
})
