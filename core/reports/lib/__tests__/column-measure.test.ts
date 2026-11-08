import { describe, expect, it } from 'vitest'
import { CARET_ROOM, INDENT } from '../../components/report-table-layout'
import {
    CELL_PADDING,
    CH_WIDTH,
    cellTextStyle,
    deepestLabelStart,
    defaultFlexGrow,
    isFigureType,
    labelChrome,
    labelNeed,
    measureColumns,
    measureKey,
    SORT_ICON_ROOM,
    TEXT_MIN_WIDTH,
    WORD_MARGIN,
} from '../column-measure'
import { cellDisplay, DEFAULT_DISPLAY } from '../format'
import type { VisibleRow } from '../rows'
import type { ColumnFit } from '../spec'
import { atWidth } from '../table-lines'
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

describe('measureKey', () => {
    const base = {
        runKey: 'r1',
        format: DEFAULT_DISPLAY,
        columns: COLUMNS,
        levels: GROUPED,
    }

    it('stays for the same run, columns and format', () => {
        expect(measureKey(base)).toBe(measureKey({ ...base, columns: [...COLUMNS] }))
    })

    it('changes with the run, a hidden column, the number format and the levels', () => {
        expect(measureKey({ ...base, runKey: 'r2' })).not.toBe(measureKey(base))
        expect(measureKey({ ...base, columns: COLUMNS.slice(0, 4) })).not.toBe(measureKey(base))
        expect(
            measureKey({ ...base, format: { ...DEFAULT_DISPLAY, divideBy1000: true } })
        ).not.toBe(measureKey(base))
        expect(measureKey({ ...base, levels: FLAT })).not.toBe(measureKey(base))
    })
})

describe('column fit in the measure', () => {
    // An untyped label, so the label column is text and takes settings.
    const NOTE = item('detail', 'Office supplies for the front desk', {
        number: 'JE-7',
        memo: 'Reversal of JE-12 for October rent',
        debit: 1200,
    })
    const rows = [GROUP, NOTE, TOTAL]
    const plain = measureColumns({
        columns: COLUMNS,
        rows,
        format: DEFAULT_DISPLAY,
        levels: GROUPED,
    })
    const withFits = (fits: Record<string, ColumnFit>) =>
        measureColumns({ columns: COLUMNS, rows, format: DEFAULT_DISPLAY, levels: GROUPED, fits })
    const ch = (n: number) => Math.ceil(n * CH_WIDTH) + CELL_PADDING
    const memo = plain.memo ?? { natural: 0, min: 0, grow: 0 }

    it('changes nothing without settings', () => {
        expect(withFits({})).toEqual(plain)
    })

    it('raises a text column to its minWidth, and its natural width with it', () => {
        const wide = withFits({ memo: { minWidth: '60ch' } }).memo
        expect(wide).toEqual({ natural: Math.max(memo.natural, ch(60)), min: ch(60), grow: 1 })
    })

    it('keeps the whole value with max-content and the longest word with min-content', () => {
        const m = withFits({
            memo: { minWidth: 'max-content' },
            number: { minWidth: 'min-content' },
        })
        expect(m.memo?.min).toBe(memo.natural)
        expect(m.number?.min).toBe(
            Math.max(
                longestWordWidth('No.', HEADING) + CELL_PADDING,
                longestWordWidth('JE-7', CELL) + CELL_PADDING
            )
        )
    })

    it('caps a column at maxWidth, never under its min', () => {
        const m = withFits({ memo: { maxWidth: '10ch' } }).memo
        expect(m?.max).toBe(Math.max(memo.min, ch(10)))
        expect(m?.natural).toBe(m?.max)
    })

    it('fixes a column at width', () => {
        expect(withFits({ memo: { width: '100px' } }).memo).toEqual({
            natural: 100 + CELL_PADDING,
            min: 100 + CELL_PADDING,
            max: 100 + CELL_PADDING,
            grow: 1,
        })
    })

    it('lets a wrapping column narrow to its longest word or eight letters, on two lines', () => {
        const m = withFits({ memo: { whiteSpace: 'normal' } }).memo
        const word = longestWordWidth('Reversal of JE-12 for October rent', CELL) + CELL_PADDING
        expect(m?.lines).toBe(2)
        expect(m?.min).toBe(
            Math.max(
                memo.min,
                Math.min(memo.natural, Math.max(word, TEXT_MIN_WIDTH + CELL_PADDING))
            )
        )
        expect(withFits({ memo: { whiteSpace: 'normal', lineClamp: 3 } }).memo?.lines).toBe(3)
    })

    describe('a wrapping column is never narrower than its longest word', () => {
        // "Incomprehensibilities" is wider than 5 letters and than the eight-letter floor.
        const LONG = 'Incomprehensibilities Wholesale'
        const LABEL_ROW = item('detail', LONG, { memo: LONG, name: LONG, debit: 1200 })
        const run = (fits: Record<string, ColumnFit>) =>
            measureColumns({
                columns: COLUMNS,
                rows: [LABEL_ROW],
                format: DEFAULT_DISPLAY,
                levels: GROUPED,
                fits,
            })
        const cellWord = longestWordWidth(LONG, CELL) + CELL_PADDING
        const labelWord = INDENT + INDENT + longestWordWidth(LONG, CELL) + CELL_PADDING / 2

        it('raises a narrow minWidth or width to the longest word, with a margin', () => {
            const wrap = { whiteSpace: 'normal' } as const
            const m = run({
                account: { ...wrap, minWidth: '5ch' },
                memo: { ...wrap, minWidth: '5ch' },
                name: { ...wrap, width: '5ch' },
            })
            expect(m.account?.min).toBe(labelWord + WORD_MARGIN)
            expect(m.memo?.min).toBe(cellWord + WORD_MARGIN)
            expect(m.name?.min).toBe(cellWord + WORD_MARGIN)
            for (const col of [m.account, m.memo, m.name]) {
                expect(col?.natural).toBeGreaterThanOrEqual(col?.min ?? 0)
                expect(col?.max ?? Infinity).toBeGreaterThanOrEqual(col?.min ?? 0)
            }
        })

        it('holds a % width, minWidth or maxWidth at the longest word', () => {
            const m = run({
                memo: { whiteSpace: 'normal', width: '1%' },
                name: { whiteSpace: 'normal', minWidth: '1%', maxWidth: '2%' },
            })
            for (const col of [m.memo, m.name]) {
                const at = atWidth(col ?? { natural: 0, min: 0, grow: 0 }, 390)
                expect(at.min).toBe(cellWord + WORD_MARGIN)
                expect(at.max ?? Infinity).toBeGreaterThanOrEqual(at.min)
            }
        })

        it('keeps a minWidth wider than the word as set', () => {
            expect(run({ memo: { whiteSpace: 'normal', minWidth: '60ch' } }).memo?.min).toBe(ch(60))
        })

        it('does not raise a one-line column: it ends in …', () => {
            expect(run({ memo: { minWidth: '5ch' } }).memo?.min).toBeLessThan(cellWord)
        })
    })

    it('keeps a % for the fit to resolve at the line width', () => {
        expect(withFits({ memo: { width: '25%' } }).memo).toEqual({ ...memo, pct: { width: 0.25 } })
        expect(withFits({ memo: { minWidth: '10%', maxWidth: '40%' } }).memo?.pct).toEqual({
            min: 0.1,
            max: 0.4,
        })
    })

    it('takes flexGrow on a text column and on a figure', () => {
        expect(withFits({ memo: { flexGrow: 3 } }).memo).toEqual({ ...memo, grow: 3 })
        expect(withFits({ debit: { flexGrow: 2 } }).debit).toEqual({ ...plain.debit, grow: 2 })
    })

    it('ignores a setting that would cut a figure (the engine refuses it)', () => {
        expect(withFits({ debit: { width: '2ch', whiteSpace: 'normal' } }).debit).toEqual(
            plain.debit
        )
    })

    it('pads a label length with the label chrome', () => {
        expect(withFits({ account: { width: '20ch' } }).account?.min).toBe(
            Math.ceil(20 * CH_WIDTH) + labelChrome(GROUPED)
        )
    })

    it('never cuts a typed label, whatever its setting', () => {
        const typed = [GROUP, DETAIL, TOTAL]
        const base = measureColumns({
            columns: COLUMNS,
            rows: typed,
            format: DEFAULT_DISPLAY,
            levels: GROUPED,
        })
        const set = measureColumns({
            columns: COLUMNS,
            rows: typed,
            format: DEFAULT_DISPLAY,
            levels: GROUPED,
            fits: { account: { width: '2ch' } },
        })
        expect(set.account).toEqual(base.account)
    })

    it('never cuts a label column of figures, even when its rows carry no label type', () => {
        const columns: ResultColumn[] = [
            { id: 'amount', label: 'Amt', type: 'money', currency: 'USD' },
            { id: 'memo', label: 'Memo', type: 'text' },
        ]
        const row = item('detail', '$12,345.67 paid in full on account', { memo: 'x' })
        const m = measureColumns({ columns, rows: [row], format: DEFAULT_DISPLAY, levels: FLAT })
        expect(m.amount?.min).toBe(labelNeed(row, DEFAULT_DISPLAY))
        expect(isFigureType('money')).toBe(true)
        expect(isFigureType('text')).toBe(false)
    })

    it('gives the label column and text a default share of 1, other figures 0', () => {
        expect(defaultFlexGrow({ isLabel: true, isFigure: true })).toBe(1)
        expect(defaultFlexGrow({ isLabel: true, isFigure: false })).toBe(1)
        expect(defaultFlexGrow({ isLabel: false, isFigure: false })).toBe(1)
        expect(defaultFlexGrow({ isLabel: false, isFigure: true })).toBe(0)
        const columns: ResultColumn[] = [
            { id: 'amount', label: 'Amt', type: 'money', currency: 'USD' },
            { id: 'debit', label: 'Debit', type: 'money', currency: 'USD' },
        ]
        const row = item('detail', '$12.00', { debit: 1200 })
        const m = measureColumns({ columns, rows: [row], format: DEFAULT_DISPLAY, levels: FLAT })
        expect(m.amount?.grow).toBe(defaultFlexGrow({ isLabel: true, isFigure: true }))
        expect(m.debit?.grow).toBe(defaultFlexGrow({ isLabel: false, isFigure: true }))
    })

    it('reads a pivot cell column’s fit from its spec column', () => {
        const columns: ResultColumn[] = [
            { id: 'account', label: 'Account', type: 'text' },
            { id: 'note_q1', label: 'Q1', type: 'text', pivot: { cell: 'note', key: 'q1' } },
            {
                id: 'amount_q1',
                label: 'Q1',
                type: 'money',
                currency: 'USD',
                pivot: { cell: 'amount', key: 'q1' },
            },
        ]
        const row = item('detail', 'Rent', { note_q1: 'Paid late', amount_q1: 1200 })
        const run = (fits?: Record<string, ColumnFit>) =>
            measureColumns({ columns, rows: [row], format: DEFAULT_DISPLAY, levels: FLAT, fits })
        const m = run({ amount: { flexGrow: 3 }, note: { minWidth: '40ch' } })
        expect(m.amount_q1?.grow).toBe(3)
        expect(m.note_q1?.min).toBe(ch(40))
        expect(run({ amount_q1: { flexGrow: 3 } }).amount_q1?.grow).toBe(0)
    })
})

describe('measureKey with fits', () => {
    it('changes with a column fit', () => {
        const base = { runKey: 'r1', format: DEFAULT_DISPLAY, columns: COLUMNS, levels: GROUPED }
        expect(measureKey({ ...base, fits: { memo: { minWidth: '12ch' } } })).not.toBe(
            measureKey(base)
        )
        expect(measureKey({ ...base, fits: {} })).toBe(measureKey(base))
    })
})
