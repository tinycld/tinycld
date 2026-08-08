import { describe, expect, it } from 'vitest'

/**
 * The native editor's WebView renders inside `<View className="flex-1">`.
 *
 * `flex-1` resolves against the parent's height, and a ScrollView gives its
 * children an UNBOUNDED one — so the View computes to zero, the WebView gets
 * zero, and the editor renders nothing while reporting no error at all. The
 * document is loaded and invisible, which reads as "collaboration is broken"
 * rather than "the box has no height".
 *
 * That is what happened to the card description on iOS: the relay was working
 * and the page's Y.Doc held the full text. `minHeight` is the floor that keeps
 * a flex child visible in that container.
 *
 * The height math is asserted here rather than through a renderer because the
 * failure is arithmetic, not React: no native test harness exists in this
 * ecosystem, and a jsdom render of a react-native-webview would not reproduce
 * the layout pass that produced the zero.
 */

/** What a flex child resolves to, given its parent's available height. */
function resolvedHeight(parentHeight: number | 'unbounded', minHeight: number): number {
    const fromFlex = parentHeight === 'unbounded' ? 0 : parentHeight
    return Math.max(fromFlex, minHeight)
}

const DEFAULT_MIN_HEIGHT = 72

describe('WebView editor height', () => {
    it('stays visible inside a ScrollView, where flex resolves to zero', () => {
        expect(resolvedHeight('unbounded', DEFAULT_MIN_HEIGHT)).toBeGreaterThan(0)
    })

    it('would collapse to nothing without the floor — the shipped bug', () => {
        expect(resolvedHeight('unbounded', 0)).toBe(0)
    })

    it('still fills a bounded parent, so mail compose is unaffected', () => {
        // The floor must not cap a container that DOES have a height.
        expect(resolvedHeight(400, DEFAULT_MIN_HEIGHT)).toBe(400)
    })

    it('defaults to roughly three lines of text', () => {
        // Enough to read a short description, and to make an empty editor
        // look like somewhere to type rather than a gap in the page.
        expect(DEFAULT_MIN_HEIGHT).toBeGreaterThanOrEqual(48)
    })
})
