// When a paged list asks for its next page: near the end of what it holds, so
// the next page is there before the reader is.

/** About ten rows before the end. */
export const NEAR_END_PX = 600

export type ScrollMetrics = {
    contentOffset: { y: number }
    layoutMeasurement: { height: number }
    contentSize: { height: number }
}

/** A page that does not fill the view counts as near the end: it never scrolls. */
export function isNearEnd({ contentOffset, layoutMeasurement, contentSize }: ScrollMetrics) {
    return contentOffset.y + layoutMeasurement.height >= contentSize.height - NEAR_END_PX
}
