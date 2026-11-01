import { describe, expect, it } from 'vitest'
import type { ResultColumn } from '../../lib/types'
import {
    AMOUNT_WIDTH,
    columnAlign,
    LABEL_MIN_WIDTH,
    PHONE_AMOUNT_WIDTH,
    PHONE_LABEL_WIDTH,
    reportTableLayout,
} from '../report-table-layout'

describe('reportTableLayout', () => {
    it('gives the label the free space on wide screens and scrolls the amounts sideways', () => {
        expect(reportTableLayout({ breakpoint: 'desktop', amountCount: 3 })).toEqual({
            labelStyle: { flex: 1, minWidth: LABEL_MIN_WIDTH },
            amountWidth: AMOUNT_WIDTH,
            scrollsSideways: true,
            amountContentMinWidth: 3 * AMOUNT_WIDTH,
        })
    })

    it('fits one or two narrower amount columns on a phone, and scrolls three or more', () => {
        expect(reportTableLayout({ breakpoint: 'mobile', amountCount: 2 })).toMatchObject({
            amountWidth: PHONE_AMOUNT_WIDTH,
            scrollsSideways: false,
            labelStyle: { flex: 1 },
        })
        expect(reportTableLayout({ breakpoint: 'mobile', amountCount: 3 })).toMatchObject({
            amountWidth: AMOUNT_WIDTH,
            scrollsSideways: true,
            labelStyle: { width: PHONE_LABEL_WIDTH },
        })
    })
})

describe('columnAlign', () => {
    const money: ResultColumn = { id: 'cost', label: 'Cost', type: 'money' }
    const text: ResultColumn = { id: 'note', label: 'Note', type: 'text' }

    it('keeps the label column left and numbers right', () => {
        expect(columnAlign(money, 0)).toBe('left')
        expect(columnAlign(money, 1)).toBe('right')
        expect(columnAlign(text, 1)).toBe('left')
        expect(columnAlign({ ...text, totalType: 'number' }, 2)).toBe('right')
    })
})
