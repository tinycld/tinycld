import { isRightAligned } from '../lib/format'
import type { ResultColumn } from '../lib/types'

export const ROW_HEIGHT = 44
/** A wrapped row's later lines: not a press target, so shorter than ROW_HEIGHT (D6). */
export const CONTINUATION_LINE_HEIGHT = 28
/** A heading line when the rows wrap: plain text, sorted from the Sort chip (D10). */
export const HEADING_LINE_HEIGHT = 24
export const INDENT = 16
/** A group row's fold caret. */
export const CARET_SIZE = 14
/** The room a group row's caret takes before its label text: the caret and its gap-1. */
export const CARET_ROOM = CARET_SIZE + 4

/** One rule for a heading and its cells. The first column holds the row labels, so it is left. */
export function columnAlign(column: ResultColumn, index: number): 'left' | 'right' {
    if (index === 0) return 'left'
    return isRightAligned(column) ? 'right' : 'left'
}
