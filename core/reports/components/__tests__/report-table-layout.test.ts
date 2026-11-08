import { describe, expect, it } from 'vitest'
import { isFigureColumn, rowLevelsOf } from '../../lib/table-lines'
import type { ResultColumn } from '../../lib/types'
import { columnAlign } from '../report-table-layout'

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
        expect(isFigureColumn({ id: 'x', label: 'X', type: 'text', totalType: 'number' })).toBe(
            true
        )
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
