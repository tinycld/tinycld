import type { Breakpoint } from '@tinycld/core/components/workspace/useBreakpoint'
import { isRightAligned } from '../lib/format'
import type { ResultColumn } from '../lib/types'

export const ROW_HEIGHT = 44
export const AMOUNT_WIDTH = 128
export const LABEL_MIN_WIDTH = 220
export const PHONE_LABEL_WIDTH = 168
export const PHONE_AMOUNT_WIDTH = 108
export const INDENT = 16

// A phone fits this many amount columns whole beside the label; more scroll sideways.
const PHONE_FIT_COLUMNS = 2

export type ReportTableLayout = {
    labelStyle: { flex?: number; minWidth?: number; width?: number }
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

export function reportTableLayout(input: {
    breakpoint: Breakpoint
    amountCount: number
}): ReportTableLayout {
    const { breakpoint, amountCount } = input
    if (breakpoint !== 'mobile') {
        return layoutFor({ flex: 1, minWidth: LABEL_MIN_WIDTH }, AMOUNT_WIDTH, true, amountCount)
    }
    if (amountCount <= PHONE_FIT_COLUMNS)
        return layoutFor({ flex: 1 }, PHONE_AMOUNT_WIDTH, false, amountCount)
    return layoutFor({ width: PHONE_LABEL_WIDTH }, AMOUNT_WIDTH, true, amountCount)
}
