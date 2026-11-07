import { describe, expect, it } from 'vitest'
import { fitOneLine, isFigureColumn, rowLevelsOf } from '../../lib/table-lines'
import type { ResultColumn } from '../../lib/types'
import {
    AMOUNT_WIDTH,
    columnAlign,
    LABEL_MIN_WIDTH,
    PHONE_AMOUNT_WIDTH,
    PHONE_LABEL_WIDTH,
    TEXT_FLOOR,
} from '../report-table-layout'

// A wide report: the label, five text columns and three amounts. Its natural line
// is 220 + 8 × 128 = 1244; narrowed to the floor it is 80 + 5 × 80 + 3 × 128 = 864.
const WIDE: ResultColumn[] = [
    { id: 'customer', label: 'Customer', type: 'text' },
    { id: 'status', label: 'Status', type: 'select' },
    { id: 'ref', label: 'Ref', type: 'text' },
    { id: 'team', label: 'Team', type: 'relation' },
    { id: 'note', label: 'Note', type: 'text' },
    { id: 'other', label: 'Other', type: 'text' },
    { id: 'amount', label: 'Amount', type: 'money', currency: 'USD' },
    { id: 'cost', label: 'Cost', type: 'money', currency: 'USD' },
    { id: 'balance', label: 'Balance', type: 'money', currency: 'USD' },
]
const TEXT_IDS = ['status', 'ref', 'team', 'note', 'other']
const wideWidths = (label: number, text: number, amount: number) => ({
    customer: label,
    ...Object.fromEntries(TEXT_IDS.map(id => [id, text] as const)),
    amount,
    cost: amount,
    balance: amount,
})

describe('fitOneLine', () => {
    it('keeps every natural width where the line fits', () => {
        expect(fitOneLine(WIDE, 1244, 'desktop')).toEqual(
            wideWidths(LABEL_MIN_WIDTH, AMOUNT_WIDTH, AMOUNT_WIDTH)
        )
        expect(fitOneLine(WIDE, 1600, 'desktop')).toEqual(
            wideWidths(LABEL_MIN_WIDTH, AMOUNT_WIDTH, AMOUNT_WIDTH)
        )
    })

    it('narrows the label first', () => {
        expect(fitOneLine(WIDE, 1200, 'desktop')).toEqual(wideWidths(176, 128, 128))
    })

    it('then lowers the text columns to one cap, the figures kept', () => {
        // 1100 − 80 − 3 × 128 = 636 for five text columns: 127 each.
        expect(fitOneLine(WIDE, 1100, 'desktop')).toEqual(wideWidths(TEXT_FLOOR, 127, 128))
        expect(fitOneLine(WIDE, 864, 'desktop')).toEqual(wideWidths(TEXT_FLOOR, TEXT_FLOOR, 128))
    })

    it('does not fit below the floor', () => {
        expect(fitOneLine(WIDE, 863, 'desktop')).toBeNull()
    })

    it('keeps a number, a percent, a date, a time and a yes/no at full width', () => {
        const columns: ResultColumn[] = [
            { id: 'customer', label: 'Customer', type: 'text' },
            { id: 'count', label: 'Count', type: 'number' },
            { id: 'share', label: 'Share', type: 'percent' },
            { id: 'day', label: 'Day', type: 'date' },
            { id: 'booked', label: 'Booked', type: 'datetime' },
            { id: 'done', label: 'Done', type: 'bool' },
            { id: 'note', label: 'Note', type: 'text' },
        ]
        // 820 − 80 − 5 × 128 = 100 for the note.
        expect(fitOneLine(columns, 820, 'desktop')).toEqual({
            customer: TEXT_FLOOR,
            count: 128,
            share: 128,
            day: 128,
            booked: 128,
            done: 128,
            note: 100,
        })
        expect(fitOneLine(columns.slice(0, 6), 719, 'desktop')).toBeNull()
    })

    it('uses the phone widths on a phone', () => {
        const columns: ResultColumn[] = [
            { id: 'customer', label: 'Customer', type: 'text' },
            { id: 'ref', label: 'Ref', type: 'text' },
            { id: 'amount', label: 'Amount', type: 'money', currency: 'USD' },
            { id: 'cost', label: 'Cost', type: 'money', currency: 'USD' },
        ]
        expect(fitOneLine(columns, 492, 'mobile')).toEqual({
            customer: PHONE_LABEL_WIDTH,
            ref: PHONE_AMOUNT_WIDTH,
            amount: PHONE_AMOUNT_WIDTH,
            cost: PHONE_AMOUNT_WIDTH,
        })
        // 390 − 80 − 2 × 108 = 94 for Ref.
        expect(fitOneLine(columns, 390, 'mobile')).toEqual({
            customer: TEXT_FLOOR,
            ref: 94,
            amount: 108,
            cost: 108,
        })
        expect(fitOneLine(columns, 375, 'mobile')).toBeNull()
    })

    it('fits a lone label down to the floor', () => {
        const label: ResultColumn[] = [{ id: 'customer', label: 'Customer', type: 'text' }]
        expect(fitOneLine(label, 100, 'desktop')).toEqual({ customer: 100 })
        expect(fitOneLine(label, 79, 'desktop')).toBeNull()
        expect(fitOneLine([], 10, 'desktop')).toEqual({})
    })

    // The label's floor is for its text: the deepest row's indent and a group
    // row's caret come on top, so a grouped report's labels keep 8 characters.
    it('raises the label floor by the indent of the deepest row of a grouped report', () => {
        // Two group levels: a detail row is indented 2 × 16 = 32, a level-2 group
        // row 16 + its caret 18 = 34; the floor is 80 + 34 = 114, and the narrowed
        // line 114 + 5 × 80 + 3 × 128 = 898.
        const grouped = { groups: 2, nests: 0 }
        expect(fitOneLine(WIDE, 1200, 'desktop', grouped)).toEqual(wideWidths(176, 128, 128))
        // 1100 − 114 − 3 × 128 = 602 for five text columns: 120 each.
        expect(fitOneLine(WIDE, 1100, 'desktop', grouped)).toEqual(wideWidths(114, 120, 128))
        expect(fitOneLine(WIDE, 898, 'desktop', grouped)).toEqual(wideWidths(114, 80, 128))
        expect(fitOneLine(WIDE, 897, 'desktop', grouped)).toBeNull()
    })

    it('counts nested child rows in the deepest indent', () => {
        // One group level and one level of child rows: a child row is indented
        // 2 × 16 = 32, more than the group row's caret (18); the floor is 112.
        const nested = { groups: 1, nests: 1 }
        expect(fitOneLine(WIDE, 896, 'desktop', nested)).toEqual(wideWidths(112, 80, 128))
        expect(fitOneLine(WIDE, 895, 'desktop', nested)).toBeNull()
        expect(fitOneLine(WIDE, 865, 'desktop', { groups: 0, nests: 1 })).toBeNull()
    })

    it('wraps a grouped phone row whose label would get less than its floor', () => {
        const columns: ResultColumn[] = [
            { id: 'customer', label: 'Customer', type: 'text' },
            { id: 'ref', label: 'Ref', type: 'text' },
            { id: 'amount', label: 'Amount', type: 'money', currency: 'USD' },
            { id: 'cost', label: 'Cost', type: 'money', currency: 'USD' },
        ]
        // One group level: the floor is 80 + 18 = 98, the narrowed line 98 + 80 +
        // 2 × 108 = 394, over 390 (a flat report fits there, at 80).
        expect(fitOneLine(columns, 390, 'mobile', { groups: 1, nests: 0 })).toBeNull()
        expect(fitOneLine(columns, 394, 'mobile', { groups: 1, nests: 0 })).toEqual({
            customer: 98,
            ref: 80,
            amount: 108,
            cost: 108,
        })
    })
})

describe('rowLevelsOf', () => {
    it('counts the group levels and the nested hops of the deepest column', () => {
        expect(rowLevelsOf(null)).toEqual({ groups: 0, nests: 0 })
        expect(
            rowLevelsOf({
                groups: [{ col: 'team' }, { col: 'customer' }],
                columns: [
                    { path: [{ field: 'team' }] },
                    {
                        path: [
                            { link: 'lines', mode: 'nest' },
                            { link: 'parts', mode: 'nest' },
                            { field: 'name' },
                        ],
                    },
                    { path: [{ link: 'lines', mode: 'nest' }, { field: 'qty' }] },
                ],
            })
        ).toEqual({ groups: 2, nests: 2 })
    })
})

describe('isFigureColumn', () => {
    it('is a column whose values an ellipsis would ruin', () => {
        for (const type of ['number', 'money', 'percent', 'date', 'datetime', 'bool'] as const)
            expect(isFigureColumn({ id: 'x', label: 'X', type })).toBe(true)
        for (const type of ['text', 'select', 'relation'] as const)
            expect(isFigureColumn({ id: 'x', label: 'X', type })).toBe(false)
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
