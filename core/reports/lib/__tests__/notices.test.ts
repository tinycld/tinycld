import { describe, expect, it } from 'vitest'
import { BAND_LINES, bandNotices, isBandCut, LAYOUT_NOTICE_CODE, layoutNoticeOf } from '../notices'
import type { LineLayout } from '../table-lines'
import { textWidth } from '../text-width'

const TEXT = { size: 'sm', weight: 'normal' } as const

describe('bandNotices', () => {
    const cap = { code: 'rowCap', message: 'The report shows its first 5000 rows.' }

    it('puts the layout notice last, and adds nothing without one', () => {
        expect(bandNotices([cap], 'Laid out automatically.')).toEqual([
            cap,
            { code: LAYOUT_NOTICE_CODE, message: 'Laid out automatically.' },
        ])
        expect(bandNotices([cap], undefined)).toEqual([cap])
    })
})

describe('layoutNoticeOf', () => {
    it('reads the notice of a wrapped layout only', () => {
        const root = { kind: 'cell', col: 'a', justify: 'start' } as const
        const blocks: LineLayout = { kind: 'blocks', root, indent: 0, notice: 'Moved.' }
        expect(layoutNoticeOf(blocks)).toBe('Moved.')
        expect(layoutNoticeOf({ ...blocks, notice: undefined })).toBeUndefined()
        expect(layoutNoticeOf({ kind: 'one-line', widths: {} })).toBeUndefined()
    })
})

describe('isBandCut', () => {
    const line = 'Rows capped here.'
    const width = textWidth(line, TEXT) + 1

    it('fits as many one-line notices as the band has lines', () => {
        expect(BAND_LINES).toBe(2)
        expect(isBandCut([line], width)).toBe(false)
        expect(isBandCut([line, line], width)).toBe(false)
        expect(isBandCut([line, line, line], width)).toBe(true)
    })

    it('counts the lines a long notice wraps to, word by word', () => {
        expect(isBandCut([`${line} ${line}`], width)).toBe(false)
        expect(isBandCut([`${line} ${line} ${line}`], width)).toBe(true)
        // A word wider than the line breaks inside itself.
        expect(isBandCut(['x'.repeat(200)], width)).toBe(true)
    })

    it('cuts nothing with no notices or no width yet', () => {
        expect(isBandCut([], width)).toBe(false)
        expect(isBandCut([line, line, line], 0)).toBe(false)
    })
})
