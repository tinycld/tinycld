import type { Breakpoint } from '@tinycld/core/components/workspace/useBreakpoint'
import { isRightAligned } from '../lib/format'
import type { ResultColumn } from '../lib/types'

export const ROW_HEIGHT = 44
export const AMOUNT_WIDTH = 128
export const LABEL_MIN_WIDTH = 220
export const PHONE_LABEL_WIDTH = 168
export const PHONE_AMOUNT_WIDTH = 108
export const INDENT = 16
/**
 * The narrowest a text column (and the label) gets on one line (R12): 8 average
 * characters at text-sm (8 × 7) plus the cell's px-3 padding on both sides.
 * Contents are not measured, so the layout does not change as pages load.
 */
export const TEXT_FLOOR = 80

export type ReportTableLayout = {
    labelStyle: { flex?: number; minWidth?: number; width?: number }
    /** The narrowest the label column gets: the table's minimum width is this plus the amounts. */
    labelMin: number
    amountWidth: number
    scrollsSideways: boolean
    amountContentMinWidth: number
}

function layoutFor(
    labelStyle: ReportTableLayout['labelStyle'],
    amountWidth: number,
    scrollsSideways: boolean,
    amountCount: number
): ReportTableLayout {
    return {
        labelStyle,
        labelMin: labelStyle.width ?? labelStyle.minWidth ?? 0,
        amountWidth,
        scrollsSideways,
        amountContentMinWidth: amountCount * amountWidth,
    }
}

/** One rule for a heading and its cells. The first column holds the row labels, so it is left. */
export function columnAlign(column: ResultColumn, index: number): 'left' | 'right' {
    if (index === 0) return 'left'
    return isRightAligned(column) ? 'right' : 'left'
}

// The one-line layout. Only a desktop scrolls sideways: a narrower screen draws
// one line only where it fits (fitOneLine) and wraps rows otherwise (E14).
export function reportTableLayout(input: {
    breakpoint: Breakpoint
    amountCount: number
}): ReportTableLayout {
    const { breakpoint, amountCount } = input
    if (breakpoint === 'mobile') {
        const label = { flex: 1, minWidth: PHONE_LABEL_WIDTH }
        return layoutFor(label, PHONE_AMOUNT_WIDTH, false, amountCount)
    }
    const label = { flex: 1, minWidth: LABEL_MIN_WIDTH }
    return layoutFor(label, AMOUNT_WIDTH, breakpoint === 'desktop', amountCount)
}
