import Decimal from '@jsamr/counter-style/presets/decimal'
import Disc from '@jsamr/counter-style/presets/disc'

/**
 * The width a list marker reserves in the READ view.
 *
 * Its own module, free of react-native-marked, so it can be unit-tested: the
 * renderer that uses it transitively pulls real react-native Flow source, which
 * Vite cannot parse.
 */

/**
 * The library's own marker-box factor — `defaultComputeMarkerBoxWidth` in
 * @jsamr/react-native-li's useMarkedList.
 */
const MARKER_BOX_EM_PER_CODEPOINT = 0.6

/**
 * Width of a bullet/number marker box, mirroring @jsamr/react-native-li's own
 * `maxNumOfCodepoints * fontSize * 0.6`.
 *
 * The codepoint count is asked of the SAME counter renderer react-native-marked
 * hands the library (`@jsamr/counter-style`'s Disc/Decimal presets), over the
 * same index range. It used to be hardcoded at two — right for a disc (`"• "`),
 * wrong for every ordered list: `"1. "` is three codepoints and `"10. "` is
 * four, so a numbered list reserved 16.8px where the library laid out 25.2px
 * and the text rendered 8.4px right of where the editor put it. Nothing caught
 * that: editor-read-parity.spec.ts only exercised a bullet list.
 *
 * Derived rather than measured at runtime because it feeds a style computed
 * during render.
 */
export function markerBoxWidth(
    ordered: boolean,
    fontSize: number,
    startIndex: number,
    length: number
): number {
    const counterRenderer = ordered ? Decimal : Disc
    // At least one item: a list still being typed has none, and an empty range
    // would ask the counter for a backwards span.
    const codepoints = counterRenderer.maxMarkerLenInRange(
        startIndex,
        startIndex + Math.max(length, 1) - 1
    )
    return codepoints * fontSize * MARKER_BOX_EM_PER_CODEPOINT
}
