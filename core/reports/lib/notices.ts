import type { LineLayout } from './table-lines'
import { textWidth } from './text-width'
import type { Notice } from './types'

/** The code of the table's own notice: an arrangement it could not follow (PIVOT_NOTICE). */
export const LAYOUT_NOTICE_CODE = 'layout'

export function layoutNoticeOf(layout: LineLayout): string | undefined {
    return layout.kind === 'blocks' ? layout.notice : undefined
}

/** H5: one band holds the run's notices and, last, the table's layout notice. */
export function bandNotices(
    notices: readonly Notice[],
    layoutNotice: string | undefined
): Notice[] {
    if (!layoutNotice) return [...notices]
    return [...notices, { code: LAYOUT_NOTICE_CODE, message: layoutNotice }]
}

/** The lines a phone's band shows before "Show all". */
export const BAND_LINES = 2

const NOTICE_TEXT = { size: 'sm', weight: 'normal' } as const

// The lines a text wraps to at this width, word by word, with textWidth's upper
// estimate: a band may offer "Show all" when its text would just fit, never the
// reverse. The line so far is measured whole (textWidth rounds up per call, so
// summing words would overcount). No layout pass, so the band draws once.
function lineCount(text: string, width: number): number {
    let lines = 1
    let line = ''
    for (const word of text.split(/\s+/).filter(Boolean)) {
        const joined = line ? `${line} ${word}` : word
        if (line && textWidth(joined, NOTICE_TEXT) > width) {
            lines += 1
            line = word
        } else {
            line = joined
        }
        // A word wider than the line breaks inside itself.
        const extra = Math.ceil(textWidth(line, NOTICE_TEXT) / width) - 1
        if (extra > 0 && line === word) lines += extra
    }
    return lines
}

/** Whether these notices, each starting a line, need more than BAND_LINES lines. */
export function isBandCut(messages: readonly string[], width: number): boolean {
    if (width <= 0) return false
    let lines = 0
    for (const message of messages) lines += lineCount(message, width)
    return lines > BAND_LINES
}
