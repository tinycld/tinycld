import { useState } from 'react'

// Mounts only the rows near the scroll position; two spacers stand in for the rest,
// so the scroll bar still spans the whole report. Near the end it asks for the next
// page.

export const ROW_OVERSCAN = 6
/** About ten rows before the end, so the next page is there before the reader is. */
export const NEAR_END_PX = 600

export type ScrollMetrics = {
    contentOffset: { y: number }
    layoutMeasurement: { height: number }
    contentSize: { height: number }
}

export function isNearEnd({
    contentOffset,
    layoutMeasurement,
    contentSize,
}: ScrollMetrics): boolean {
    return contentOffset.y + layoutMeasurement.height >= contentSize.height - NEAR_END_PX
}

export function rowWindowBounds(input: {
    firstVisible: number
    total: number
    rowHeight: number
    viewHeight: number
}) {
    const { firstVisible, total, rowHeight, viewHeight } = input
    const visibleCount = Math.ceil(viewHeight / rowHeight)
    // The upper clamp keeps the window from going empty when the list shrinks under a stale scroll position.
    const first = Math.min(
        Math.max(0, firstVisible - ROW_OVERSCAN),
        Math.max(0, total - visibleCount)
    )
    const last = Math.min(total, first + visibleCount + ROW_OVERSCAN * 2)
    return { first, last, topSpace: first * rowHeight, bottomSpace: (total - last) * rowHeight }
}

export function useRowWindow(input: {
    total: number
    rowHeight: number
    viewHeight: number
    onNearEnd?: () => void
}) {
    // The row index, not the pixel offset, so scrolling within one row causes no render.
    const [firstVisible, setFirstVisible] = useState(0)
    return {
        ...rowWindowBounds({
            firstVisible,
            total: input.total,
            rowHeight: input.rowHeight,
            viewHeight: input.viewHeight,
        }),
        onScroll: (event: { nativeEvent: ScrollMetrics }) => {
            setFirstVisible(Math.floor(event.nativeEvent.contentOffset.y / input.rowHeight))
            if (input.onNearEnd && isNearEnd(event.nativeEvent)) input.onNearEnd()
        },
    }
}
