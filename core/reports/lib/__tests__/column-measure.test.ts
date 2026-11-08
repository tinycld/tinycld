import { describe, expect, it } from 'vitest'
import { CARET_ROOM, INDENT } from '../../components/report-table-layout'
import {
    CELL_PADDING,
    cellTextStyle,
    deepestLabelStart,
    labelChrome,
    labelNeed,
    measureColumns,
    SORT_ICON_ROOM,
    TEXT_MIN_WIDTH,
} from '../column-measure'
import { cellDisplay, DEFAULT_DISPLAY } from '../format'
import type { VisibleRow } from '../rows'
import { longestWordWidth, textWidth } from '../text-width'
import type { CellValue, ResultColumn, ResultRow } from '../types'

const CELL = { size: 'sm', weight: 'normal' } as const
const BOLD = { size: 'sm', weight: 'semibold' } as const
const HEADING = { size: 'xs', weight: 'semibold', uppercase: true } as const
const FLAT = { groups: 0, nests: 0 }
const GROUPED = { groups: 1, nests: 0 }

const COLUMNS: ResultColumn[] = [
    // Short headings for the label and memo, so their rows, not their headings, size them.
    { id: 'account', label: 'Date', type: 'text' },
    { id: 'number', label: 'No.', type: 'text' },
    { id: 'memo', label: 'Memo', type: 'text' },
    // A heading wider than the text floor: its longest word is its min.
    { id: 'name', label: 'Customer name', type: 'text' },
    { id: 'debit', label: 'Debit', type: 'money', currency: 'USD', sortable: true },
]

function item(
    kind: ResultRow['kind'],
    label: string,
    cells: Record<string, CellValue>,
    extra: Partial<ResultRow> = {},
    isToggle = false
): VisibleRow {
    const row: ResultRow = { id: `${kind}:${label}`, kind, depth: 1, label, cells, ...extra }
    return { row, cells, isToggle, isOpen: isToggle }
}

const DETAIL = item(
    'detail',
    '2026-10-02',
    { number: 'JE-41', memo: 'Reversal of JE-12 for October rent', debit: 123456 },
    { labelType: 'date', labelValue: '2026-10-02' }
)
const GROUP = item('group', '1000 Business Checking', {}, { depth: 0 }, true)
const TOTAL = item('total', 'Total', { debit: 3510000 }, { depth: 0 })

const measure = (rows: VisibleRow[], levels = GROUPED) =>
    measureColumns({ columns: COLUMNS, rows, format: DEFAULT_DISPLAY, levels })

describe('measureColumns', () => {
    it('sizes every column by its heading when there are no rows', () => {
        const m = measure([], FLAT)
        // A heading may wrap at its words: "CUSTOMER" (103) is more than the text floor (93).
        expect(m.name).toEqual({
            natural: textWidth('Customer name', HEADING) + CELL_PADDING,
            min: longestWordWidth('Customer name', HEADING) + CELL_PADDING,
            grow: 1,
        })
        // A sortable heading keeps room for its icon.
        expect(m.debit.natural).toBe(textWidth('Debit', HEADING) + SORT_ICON_ROOM + CELL_PADDING)
        const date = textWidth('Date', HEADING) + CELL_PADDING
        expect(m.account).toEqual({ natural: date, min: date, grow: 1 })
    })

    it('never cuts a figure: its min is its widest value, Total rows in semibold included', () => {
        const m = measure([DETAIL, TOTAL])
        const debit = COLUMNS[4] as ResultColumn
        const total = textWidth(cellDisplay(debit, TOTAL.row, 3510000, DEFAULT_DISPLAY).text, BOLD)
        const worst = textWidth(
            cellDisplay(debit, DETAIL.row, -9_999_999, DEFAULT_DISPLAY).text,
            CELL
        )
        expect(m.debit.natural).toBe(Math.max(total, worst) + CELL_PADDING)
        expect(m.debit.min).toBe(m.debit.natural)
        expect(m.debit.grow).toBe(0)
    })

    it('keeps the figure worst case when the first rows are short', () => {
        const small = item(
            'detail',
            '2026-10-02',
            { debit: 500 },
            { labelType: 'date', labelValue: '2026-10-02' }
        )
        const worst = (format: typeof DEFAULT_DISPLAY) =>
            textWidth(
                cellDisplay(COLUMNS[4] as ResultColumn, small.row, -9_999_999, format).text,
                CELL
            ) + CELL_PADDING
        expect(measure([small]).debit.natural).toBe(worst(DEFAULT_DISPLAY))
        // With parentheses for negatives the worst case is one character wider.
        const parens = { ...DEFAULT_DISPLAY, negative: 'parens' as const }
        const m = measureColumns({
            columns: COLUMNS,
            rows: [small],
            format: parens,
            levels: GROUPED,
        })
        expect(m.debit.natural).toBe(worst(parens))
        expect(worst(parens)).toBeGreaterThan(worst(DEFAULT_DISPLAY))
    })

    it('never cuts short text, and holds long text to eight characters at the least', () => {
        const m = measure([DETAIL])
        expect(m.number.natural).toBe(textWidth('JE-41', CELL) + CELL_PADDING)
        expect(m.number.min).toBe(m.number.natural)
        expect(m.memo.natural).toBe(
            textWidth('Reversal of JE-12 for October rent', CELL) + CELL_PADDING
        )
        expect(m.memo.min).toBe(TEXT_MIN_WIDTH + CELL_PADDING)
        expect(m.memo.grow).toBe(1)
    })

    it('shrinks an all-empty column to its heading', () => {
        const m = measure([DETAIL, GROUP, TOTAL])
        expect(m.name.natural).toBe(textWidth('Customer name', HEADING) + CELL_PADDING)
    })

    it('sizes the label by its detail rows, and never cuts a typed date label', () => {
        const m = measure([DETAIL, GROUP])
        const dateNeed = INDENT + INDENT + textWidth('Sep 30, 2026', CELL) + CELL_PADDING / 2
        expect(m.account.natural).toBe(dateNeed)
        expect(m.account.min).toBe(dateNeed)
        // The open group row spans the cells after it, so it does not size the column.
        expect(m.account.natural).toBeLessThan(labelNeed(GROUP, DEFAULT_DISPLAY))
    })

    it('counts a non-detail row whose first other cell has a value: it spans nothing', () => {
        const closed = item(
            'group',
            '1000 Business Checking',
            { number: '1000' },
            { depth: 0 },
            true
        )
        expect(measure([closed]).account.natural).toBe(labelNeed(closed, DEFAULT_DISPLAY))
    })

    it('lets a text label narrow to its chrome and eight characters', () => {
        const text = item('detail', 'Harbor Lane Design LLC, Accounts receivable', {})
        const m = measure([text])
        expect(m.account.min).toBe(labelChrome(GROUPED) + TEXT_MIN_WIDTH)
        expect(m.account.grow).toBe(1)
    })
})

describe('the label chrome', () => {
    it('is the base indent, the deepest label start and pr-3', () => {
        // One group level: a detail row at 16, a group row's caret at 18.
        expect(deepestLabelStart(GROUPED)).toBe(CARET_ROOM)
        expect(labelChrome(GROUPED)).toBe(INDENT + CARET_ROOM + CELL_PADDING / 2)
        expect(labelChrome(FLAT)).toBe(INDENT + CELL_PADDING / 2)
    })

    it('counts a toggle row’s caret in its label need', () => {
        expect(labelNeed(GROUP, DEFAULT_DISPLAY)).toBe(
            INDENT + CARET_ROOM + textWidth('1000 Business Checking', BOLD) + CELL_PADDING / 2
        )
        expect(cellTextStyle('detail')).toEqual(CELL)
        expect(cellTextStyle('subtotal')).toEqual(BOLD)
    })
})
