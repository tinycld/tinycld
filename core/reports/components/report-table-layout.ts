import { isRightAligned } from '../lib/format'
import type { ResultColumn } from '../lib/types'

export const ROW_HEIGHT = 44
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
