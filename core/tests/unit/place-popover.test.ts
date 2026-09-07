import { placePopover, placeSubmenu } from '@tinycld/core/ui/popover/place'
import { describe, expect, it } from 'vitest'

const viewport = { width: 1000, height: 600 }
const anchor = { x: 400, y: 100, width: 80, height: 30 }
const size = { width: 200, height: 150 }

describe('placePopover', () => {
    it('sits below the anchor, aligned to its start, by default', () => {
        const result = placePopover({ anchor, size, viewport })
        expect(result.side).toBe('bottom')
        expect(result.left).toBe(400)
        expect(result.top).toBe(100 + 30 + 4)
    })

    it('aligns end and center on the anchor', () => {
        expect(placePopover({ anchor, size, viewport, placement: 'bottom-end' }).left).toBe(
            400 + 80 - 200
        )
        expect(placePopover({ anchor, size, viewport, placement: 'bottom-center' }).left).toBe(
            400 + 40 - 100
        )
    })

    it('flips above when there is no room below and enough above', () => {
        const low = { ...anchor, y: 500 }
        const result = placePopover({ anchor: low, size, viewport })
        expect(result.side).toBe('top')
        expect(result.top).toBe(500 - 4 - 150)
    })

    it('stays on the side with more room and caps the height when neither side fits', () => {
        const tall = { width: 200, height: 900 }
        const result = placePopover({ anchor, size: tall, viewport })
        // 100px above, 466px below: below wins, capped to what is there.
        expect(result.side).toBe('bottom')
        expect(result.maxHeight).toBe(600 - 130 - 4 - 8)
        // The capped surface still ends inside the viewport.
        expect(result.top + result.maxHeight).toBeLessThanOrEqual(600 - 8)
    })

    it('clamps a start-aligned surface off the right edge back inside', () => {
        const nearEdge = { ...anchor, x: 900 }
        const result = placePopover({ anchor: nearEdge, size, viewport })
        expect(result.left).toBe(1000 - 200 - 8)
    })

    it('clamps an end-aligned surface off the left edge back inside', () => {
        const nearEdge = { ...anchor, x: 20 }
        const result = placePopover({ anchor: nearEdge, size, viewport, placement: 'bottom-end' })
        expect(result.left).toBe(8)
    })

    it('opens to the right of the anchor and flips left at the edge', () => {
        const right = placePopover({ anchor, size, viewport, placement: 'right' })
        expect(right.side).toBe('right')
        expect(right.left).toBe(400 + 80 + 4)
        expect(right.top).toBe(100)

        const nearEdge = { ...anchor, x: 900 }
        const flipped = placePopover({ anchor: nearEdge, size, viewport, placement: 'right' })
        expect(flipped.side).toBe('left')
        expect(flipped.left).toBe(900 - 4 - 200)
    })

    it('treats an unmeasured surface as zero-sized rather than guessing', () => {
        const result = placePopover({ anchor, size: null, viewport })
        expect(result.left).toBe(400)
        expect(result.top).toBe(134)
    })

    it('places from a point anchor for a context menu', () => {
        const point = { x: 650, y: 300, width: 0, height: 0 }
        const result = placePopover({ anchor: point, size, viewport })
        expect(result.left).toBe(650)
        expect(result.top).toBe(304)
    })
})

describe('placeSubmenu', () => {
    const parent = { x: 300, y: 100, width: 220, height: 300 }
    const row = { x: 300, y: 180, width: 220, height: 36 }

    it('opens beside the parent, level with its row, overlapping the edge', () => {
        const result = placeSubmenu({ parent, row, size, viewport })
        expect(result.left).toBe(300 + 220 - 4)
        expect(result.top).toBe(180 - 4)
    })

    it('flips to the parent’s left when the right edge is in the way', () => {
        const nearEdge = { ...parent, x: 700 }
        const result = placeSubmenu({ parent: nearEdge, row: { ...row, x: 700 }, size, viewport })
        expect(result.left).toBe(700 - 200 + 4)
    })

    it('rises to stay inside the bottom edge', () => {
        const lowRow = { ...row, y: 560 }
        const result = placeSubmenu({ parent, row: lowRow, size, viewport })
        expect(result.top).toBe(600 - 150 - 8)
    })
})
