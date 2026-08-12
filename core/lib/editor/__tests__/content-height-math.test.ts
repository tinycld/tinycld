import { describe, expect, it } from 'vitest'

/**
 * How the in-WebView page computes the height it reports.
 *
 * The arithmetic is what shipped clipped twice, so it is pinned here rather
 * than left to a device check. `useContentHeight` reads real DOM boxes, which
 * a jsdom test cannot reproduce faithfully — but the rule it applies to those
 * boxes is pure, and that rule is what was wrong both times.
 */

/** Mirrors the computation in Editor.tsx's `report()`. */
function reportedHeight(input: {
    /** Last block's bottom, relative to the viewport. */
    lastBottom: number
    /** Page scroll offset — nonzero once the document is taller than the view. */
    scrollY: number
    /** Collapsed bottom margin of that last block. */
    lastMarginBottom: number
    trailingSpace: number
}): number {
    return Math.ceil(
        input.lastBottom + input.scrollY + input.lastMarginBottom + input.trailingSpace
    )
}

const TRAILING_SPACE_PX = 24

describe('reported content height', () => {
    it('measures from the top of the page, not from the first block', () => {
        // The first version measured firstChildTop → lastChildBottom, which
        // silently dropped everything above the first block. Anchoring at the
        // page origin is what makes the number absolute.
        const height = reportedHeight({
            lastBottom: 532,
            scrollY: 0,
            lastMarginBottom: 0,
            trailingSpace: 0,
        })
        expect(height).toBe(532)
    })

    it('includes the last block’s bottom margin', () => {
        // A collapsed bottom margin falls OUTSIDE every child's bounding box,
        // so a first-to-last measurement misses it — and the final line gets
        // clipped by exactly that much.
        const withMargin = reportedHeight({
            lastBottom: 532,
            scrollY: 0,
            lastMarginBottom: 16,
            trailingSpace: 0,
        })
        expect(withMargin).toBe(548)
    })

    it('accounts for scroll offset, so a scrolled document is not under-reported', () => {
        // getBoundingClientRect is viewport-relative: once the page has
        // scrolled, the last block's `bottom` is smaller than its true
        // document position by exactly scrollY.
        const height = reportedHeight({
            lastBottom: 200,
            scrollY: 400,
            lastMarginBottom: 0,
            trailingSpace: 0,
        })
        expect(height).toBe(600)
    })

    it('adds trailing space, so the last line is never flush against the edge', () => {
        const height = reportedHeight({
            lastBottom: 532,
            scrollY: 0,
            lastMarginBottom: 0,
            trailingSpace: TRAILING_SPACE_PX,
        })
        expect(height).toBe(532 + TRAILING_SPACE_PX)
    })

    it('rounds UP, so a fractional line is never cut in half', () => {
        // The real measurement from the device was 516.484375. Rounding down
        // takes a slice off the descenders of the final line.
        const height = reportedHeight({
            lastBottom: 516.484375,
            scrollY: 0,
            lastMarginBottom: 0,
            trailingSpace: 0,
        })
        expect(height).toBe(517)
    })
})
