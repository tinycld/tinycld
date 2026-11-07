import { isRightAligned } from '../lib/format'
import type { ResultColumn } from '../lib/types'

export const ROW_HEIGHT = 44
export const AMOUNT_WIDTH = 128
export const LABEL_MIN_WIDTH = 220
export const PHONE_LABEL_WIDTH = 168
export const PHONE_AMOUNT_WIDTH = 108
export const INDENT = 16
/** A group row's fold caret. */
export const CARET_SIZE = 14
/** The room a group row's caret takes before its label text: the caret and its gap-1. */
export const CARET_ROOM = CARET_SIZE + 4
/**
 * The narrowest a text column (and the label's text) gets on one line (R12): 8
 * average characters at text-sm (8 × 7) plus the cell's px-3 padding on both
 * sides. The label column adds its deepest row's indent on top (labelFloor).
 * Contents are not measured, so the layout does not change as pages load.
 */
export const TEXT_FLOOR = 80

/** One rule for a heading and its cells. The first column holds the row labels, so it is left. */
export function columnAlign(column: ResultColumn, index: number): 'left' | 'right' {
    if (index === 0) return 'left'
    return isRightAligned(column) ? 'right' : 'left'
}
