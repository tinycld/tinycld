import { describe, expect, it } from 'vitest'
import { z } from 'zod'
import { CARET_ROOM } from '../../components/report-table-layout'
import {
    type ColumnMeasure,
    type ColumnMeasures,
    deepestLabelStart,
    labelChrome,
    measureColumns,
    type RowLevels,
    TEXT_MIN_WIDTH,
} from '../column-measure'
import table from '../fixtures/display-cells.json'
import { DEFAULT_DISPLAY } from '../format'
import type { VisibleRow } from '../rows'
import { type DisplayCell, type DisplayLayout, displayCellSchema } from '../spec'
import {
    ARRANGEMENT_NOTICE,
    atWidth,
    type Block,
    continuationIndent,
    fitOneLine,
    fitTable,
    isFigureColumn,
    type LineLayout,
    labelSpans,
    measureOf,
    oneLineLabelSpan,
    oneLineLabelWidth,
    PINWHEEL_MESSAGE,
    PIVOT_NOTICE,
    placeCells,
    rowLayout,
    rowLevelsOf,
    shareLineWidth,
    shareWidth,
    startingLayout,
    tableLines,
} from '../table-lines'
import type { CellValue, ResultColumn, ResultRow } from '../types'

const COLS: ResultColumn[] = [
    { id: 'account', label: 'Account', type: 'text' },
    { id: 'number', label: 'No.', type: 'text' },
    { id: 'debit', label: 'Debit', type: 'money', currency: 'USD' },
    { id: 'credit', label: 'Credit', type: 'money', currency: 'USD' },
]

const m = (min: number, natural: number, grow: number) => ({ min, natural, grow })

// The tree of a layout without its px widths and indent.
function shapeOf(layout: LineLayout): unknown {
    const strip = (block: Block): Block => {
        if (block.kind === 'stack') return { ...block, parts: block.parts.map(strip) }
        if (block.kind !== 'side') return block
        return {
            ...block,
            parts: block.parts.map(({ block: b, tracks }) => ({ block: strip(b), tracks })),
        }
    }
    if (layout.kind === 'one-line') return layout
    const { indent: _indent, ...rest } = layout
    return { ...rest, root: strip(layout.root) }
}

const uniform = (columns: readonly ResultColumn[], min: number): ColumnMeasures =>
    Object.fromEntries(columns.map(c => [c.id, m(min, min, isFigureColumn(c) ? 0 : 1)]))

describe('shareWidth', () => {
    const parts = [m(100, 200, 1), m(50, 50, 0), m(80, 120, 1)]

    it('fits the mins exactly, and refuses less', () => {
        expect(shareWidth(parts, 230)).toEqual([100, 50, 80])
        expect(shareWidth(parts, 229)).toBeNull()
    })

    it('gives spare room in proportion to what each part lacks, the remainder to the first', () => {
        // 50 spare over lacks 100 and 40: floor(35.7) = 35, floor(14.3) = 14; 1 left.
        expect(shareWidth(parts, 280)).toEqual([136, 50, 94])
        expect(shareWidth(parts, 370)).toEqual([200, 50, 120])
    })

    it('then gives the rest by grow', () => {
        expect(shareWidth(parts, 470)).toEqual([250, 50, 170])
        expect(shareWidth([m(10, 10, 0), m(20, 20, 0)], 50)).toEqual([30, 20])
    })

    it('handles no parts and parts with no width', () => {
        expect(shareWidth([], 100)).toEqual([])
        expect(shareWidth([m(0, 0, 0), m(0, 0, 0)], 10)).toEqual([10, 0])
    })
})

// Module scope: the per-width tests below reuse it.
const COLS_MEASURES: ColumnMeasures = {
    account: m(120, 180, 1),
    number: m(64, 64, 1),
    debit: m(110, 110, 0),
    credit: m(110, 110, 0),
}

// One long text label, so a label's min is its chrome and eight letters, not its heading.
const LONG_LABEL_ROW: VisibleRow = {
    row: {
        id: 'detail:1',
        kind: 'detail',
        depth: 0,
        label: 'Harbor Lane Design LLC, Accounts receivable',
        cells: {},
    },
    cells: {},
    isToggle: false,
    isOpen: false,
}
const labelMinOf = (levels: RowLevels) =>
    measureColumns({ columns: COLS, rows: [LONG_LABEL_ROW], format: DEFAULT_DISPLAY, levels })
        .account?.min ?? 0

describe('fitOneLine by content', () => {
    const measures = COLS_MEASURES

    // J2 with review A NOTE 1: the spare room goes by grow (account and number, 1
    // each), but number is already at its natural width (64) while account lacks
    // 60, so number takes no room until account has its natural width; the rest
    // then goes by grow. (Under J2 alone it was 168 / 112 at 500 and 130 / 74 at
    // 424: number passed its natural width while account was cut.)
    it('gives each column its min and the spare room to the text by grow', () => {
        // Mins 404; 96 spare: account's 60 first, then 36 by grow, 18 each.
        expect(fitOneLine(COLS, 500, measures)).toEqual({
            account: 198,
            number: 82,
            debit: 110,
            credit: 110,
        })
    })

    it('narrows the text toward its min before the line breaks', () => {
        // 20 spare, all to account: number is at its natural width.
        expect(fitOneLine(COLS, 424, measures)).toEqual({
            account: 140,
            number: 64,
            debit: 110,
            credit: 110,
        })
        expect(fitOneLine(COLS, 403, measures)).toBeNull()
        expect(fitOneLine([], 10, measures)).toEqual({})
    })

    it('reads an unknown column as needing nothing', () => {
        expect(measureOf(measures, 'nope')).toEqual({ natural: 0, min: 0, grow: 0 })
    })
})

// J2 (human, 2026-11-08): one line shares its spare room by Extra room (flexGrow),
// not by what each column lacks of its natural width.
describe('one line shares its spare room by Extra room', () => {
    const COLS_J2: ResultColumn[] = [
        { id: 'name', label: 'Name', type: 'text' },
        { id: 'description', label: 'Description', type: 'text' },
        { id: 'amount', label: 'Amount', type: 'money', currency: 'USD' },
    ]

    it('gives a Description with flexGrow 2 more of the spare room than a Name with flexGrow 1, also when Name is wider', () => {
        // Mins 80 + 80 + 100 = 260; 120 spare by grow: 40 and 80. Both still lack
        // room at 380 (was 480, where Description passed its natural 165 while Name
        // was cut: the case below refuses that, review A NOTE 1).
        const wideName = { name: m(80, 300, 1), description: m(80, 165, 2), amount: m(100, 100, 0) }
        const narrowName = {
            name: m(80, 120, 1),
            description: m(80, 300, 2),
            amount: m(100, 100, 0),
        }
        for (const measures of [wideName, narrowName]) {
            const widths = fitOneLine(COLS_J2, 380, measures)
            expect(widths).toEqual({ name: 120, description: 160, amount: 100 })
            const spare = (id: 'name' | 'description') => (widths?.[id] ?? 0) - 80
            expect(spare('description')).toBeGreaterThan(spare('name'))
        }
    })

    // Review A NOTE 1 (reports-wrap at 1100 px): "Wrapco Longname 00" needs 142 px
    // and got 124 while the currency column held 166 px for "USD". A growing column
    // stops at its natural width while another growing column is below its own; the
    // room it would take goes to the others by grow. Once every column has its
    // natural width, the rest goes by grow again, with no cap.
    it('cuts no long text while a short growing column takes room past its natural width', () => {
        const COLS_USD: ResultColumn[] = [
            { id: 'customer', label: 'Customer', type: 'text' },
            { id: 'currency', label: 'Currency', type: 'text' },
            { id: 'amount', label: 'Amount', type: 'money', currency: 'USD' },
        ]
        const measures = {
            customer: m(80, 142, 1),
            currency: m(40, 40, 1),
            amount: m(100, 100, 0),
        }
        // 50 spare: by grow alone 25 each would leave the customer cut at 105 and
        // the currency 25 past its natural width. The currency stays at 40, so the
        // customer takes all 50.
        expect(fitOneLine(COLS_USD, 270, measures)).toEqual({
            customer: 130,
            currency: 40,
            amount: 100,
        })
        // 100 spare: the customer reaches its natural 142 (62), the currency stays
        // at 40; the 38 left go by grow, 19 each.
        expect(fitOneLine(COLS_USD, 320, measures)).toEqual({
            customer: 161,
            currency: 59,
            amount: 100,
        })
    })

    it('gives a column with no Extra room its content width when there is room, and no more', () => {
        const parts = [m(80, 300, 1), m(60, 140, 0), m(100, 100, 0)]
        // Room for all: the None column takes its 80 of lack, the rest goes by grow.
        expect(shareLineWidth(parts, 600)).toEqual([360, 140, 100])
        // 40 spare: the None column's lack comes first (it gets no other room).
        expect(shareLineWidth(parts, 280)).toEqual([80, 100, 100])
        expect(shareLineWidth(parts, 239)).toBeNull()
    })
})

describe('placing cells (the engine rules)', () => {
    it('flows cells into the first free tracks of their line', () => {
        expect(
            placeCells(3, [
                { col: 'a', line: 1, span: 3 },
                { col: 'b', line: 2 },
                { col: 'c', line: 2 },
            ])
        ).toEqual({
            placed: [
                { start: 1, span: 3, line: 1, lines: 1 },
                { start: 1, span: 1, line: 2, lines: 1 },
                { start: 2, span: 1, line: 2, lines: 1 },
            ],
        })
    })
    it('names a cell with no room', () => {
        expect(
            placeCells(2, [
                { col: 'a', line: 1, span: 2 },
                { col: 'b', line: 1 },
            ])
        ).toEqual({ problem: { index: 1, message: 'no room on line 1' } })
    })
    it('lets a cell span lines anywhere a straight cut can free it, and refuses a pinwheel', () => {
        const mid = [
            { col: 'a', line: 1 },
            { col: 'm', line: 1, start: 2, lines: 2 },
            { col: 'b', line: 1, start: 3 },
            { col: 'c', line: 2, start: 1 },
            { col: 'd', line: 2, start: 3 },
        ]
        expect('placed' in placeCells(3, mid)).toBe(true)
        const pinwheel = [
            { col: 'a', line: 1, start: 1, span: 2 },
            { col: 'b', line: 1, start: 3, lines: 2 },
            { col: 'c', line: 3, start: 2, span: 2 },
            { col: 'd', line: 2, start: 1, lines: 2 },
            { col: 'e', line: 2, start: 2 },
        ]
        expect(placeCells(3, pinwheel)).toEqual({
            problem: { index: 0, message: PINWHEEL_MESSAGE },
        })
    })
})

// The table the engine runs too (core/server/reports/display_cases_test.go):
// both sides must place, refuse and cut every case the same way.
const placedSchema = z.object({
    start: z.number(),
    span: z.number(),
    line: z.number(),
    lines: z.number(),
})
const caseSchema = z.object({
    name: z.string(),
    tracks: z.number(),
    cells: z.array(displayCellSchema),
    placed: z.array(placedSchema).optional(),
    problem: z.object({ index: z.number(), message: z.string() }).optional(),
})
const SHARED = z.array(caseSchema).parse(table.cases)

describe('the shared display cell table', () => {
    // The client alone places cells (the engine checks each cell's numbers only).
    it('holds every case', () => {
        expect(SHARED.length).toBe(21)
    })
    for (const c of SHARED) {
        it(c.name, () => {
            const want = c.problem ? { problem: c.problem } : { placed: c.placed }
            expect(placeCells(c.tracks, c.cells)).toEqual(want)
        })
    }
})

// A label whose min alone (300) leaves no room for the rest at 360 or 300.
const WIDE_LABEL_MEASURES: ColumnMeasures = {
    account: m(300, 300, 1),
    number: m(64, 64, 1),
    debit: m(110, 110, 0),
    credit: m(110, 110, 0),
}

describe('the table layout per width', () => {
    it('draws one line with its natural widths wherever the columns fit', () => {
        const measures = COLS_MEASURES
        expect(tableLines({ columns: COLS, width: 1280, layouts: [], measures })).toEqual({
            kind: 'one-line',
            widths: fitOneLine(COLS, 1280, measures),
        })
        expect(tableLines({ columns: COLS, width: 900, layouts: [], measures })).toEqual({
            kind: 'one-line',
            widths: fitOneLine(COLS, 900, measures),
        })
    })
    it('wraps a phone row: the label on line 1, as many columns as fit on each line below', () => {
        // The label's min (300) leaves no room beside it; the rest fit 360 − 4.
        expect(
            shapeOf(
                tableLines({
                    columns: COLS,
                    width: 360,
                    layouts: [],
                    measures: WIDE_LABEL_MEASURES,
                })
            )
        ).toEqual({
            kind: 'blocks',
            root: {
                kind: 'stack',
                parts: [
                    {
                        kind: 'side',
                        of: 1,
                        parts: [
                            {
                                block: { kind: 'cell', col: 'account', justify: 'start' },
                                tracks: 1,
                            },
                        ],
                    },
                    {
                        kind: 'side',
                        of: 3,
                        parts: [
                            { block: { kind: 'cell', col: 'number', justify: 'start' }, tracks: 1 },
                            { block: { kind: 'cell', col: 'debit', justify: 'end' }, tracks: 1 },
                            { block: { kind: 'cell', col: 'credit', justify: 'end' }, tracks: 1 },
                        ],
                    },
                ],
            },
        })
    })
    it('packs the columns onto lines by content, a column that does not fit starting the next line', () => {
        // 110 each at 300: account and number on line 1 (220), debit and credit below.
        expect(
            shapeOf(
                tableLines({ columns: COLS, width: 300, layouts: [], measures: uniform(COLS, 110) })
            )
        ).toEqual({
            kind: 'blocks',
            root: {
                kind: 'stack',
                parts: [
                    {
                        kind: 'side',
                        of: 2,
                        parts: [
                            {
                                block: { kind: 'cell', col: 'account', justify: 'start' },
                                tracks: 1,
                            },
                            { block: { kind: 'cell', col: 'number', justify: 'start' }, tracks: 1 },
                        ],
                    },
                    {
                        kind: 'side',
                        of: 2,
                        parts: [
                            { block: { kind: 'cell', col: 'debit', justify: 'end' }, tracks: 1 },
                            { block: { kind: 'cell', col: 'credit', justify: 'end' }, tracks: 1 },
                        ],
                    },
                ],
            },
        })
    })
    // COLS at 110 each: one line needs 440, so an arrangement is tried below that.
    const COLS_110 = uniform(COLS, 110)
    it('cuts an arrangement with a mid-row cell spanning two lines into side blocks', () => {
        const layout = tableLines({
            columns: COLS,
            width: 400,
            measures: COLS_110,
            layouts: [
                {
                    width: 'mobile',
                    tracks: 3,
                    cells: [
                        { col: 'account', line: 1 },
                        { col: 'number', line: 1, start: 2, lines: 2, justify: 'center' },
                        { col: 'debit', line: 1, start: 3 },
                        { col: 'credit', line: 2, start: 3 },
                    ],
                },
            ],
        })
        expect(shapeOf(layout)).toEqual({
            kind: 'blocks',
            root: {
                kind: 'side',
                of: 3,
                parts: [
                    {
                        block: {
                            kind: 'stack',
                            parts: [
                                { kind: 'cell', col: 'account', justify: 'start' },
                                { kind: 'gap', lines: 1 },
                            ],
                        },
                        tracks: 1,
                    },
                    { block: { kind: 'cell', col: 'number', justify: 'center' }, tracks: 1 },
                    {
                        block: {
                            kind: 'stack',
                            parts: [
                                { kind: 'cell', col: 'debit', justify: 'end' },
                                { kind: 'cell', col: 'credit', justify: 'end' },
                            ],
                        },
                        tracks: 1,
                    },
                ],
            },
        })
    })
    it('puts a label the arrangement leaves out on a line of its own', () => {
        const layout = tableLines({
            columns: COLS,
            width: 400,
            measures: COLS_110,
            layouts: [{ width: 'mobile', tracks: 2, cells: [{ col: 'debit', line: 1, span: 2 }] }],
        })
        expect(shapeOf(layout)).toEqual({
            kind: 'blocks',
            root: {
                kind: 'stack',
                parts: [
                    { kind: 'cell', col: 'account', justify: 'start' },
                    { kind: 'cell', col: 'debit', justify: 'end' },
                ],
            },
        })
    })
    it('uses the first arrangement that fits, whatever the screen, and skips a refused one', () => {
        const mobile = {
            width: 'mobile' as const,
            tracks: 2,
            cells: [{ col: 'debit', line: 1, span: 2 }],
        }
        const lines = (width: number, layouts: DisplayLayout[]) =>
            tableLines({ columns: COLS, width, layouts, measures: COLS_110 })
        expect(lines(1280, [mobile])).toMatchObject({ kind: 'one-line' })
        const refused = { width: 'desktop' as const, tracks: 2, cells: [{ col: 'debit', line: 9 }] }
        // The refused desktop arrangement falls through to the phone one…
        expect(shapeOf(lines(360, [refused, mobile]))).toEqual({
            kind: 'blocks',
            root: {
                kind: 'stack',
                parts: [
                    { kind: 'cell', col: 'account', justify: 'start' },
                    { kind: 'cell', col: 'debit', justify: 'end' },
                ],
            },
        })
        // …and with nothing after it, to the automatic lines.
        const auto = lines(360, [])
        expect(auto.kind).toBe('blocks')
        expect(lines(360, [refused])).toStrictEqual({ ...auto, notice: ARRANGEMENT_NOTICE })
    })
    it('places a pivot cell column as all its pivot columns, one per track from its position', () => {
        const columns: ResultColumn[] = [
            { id: 'account', label: 'Account', type: 'text' },
            { id: 'amount_q1', label: 'Q1', type: 'money', pivot: { cell: 'amount', key: 1 } },
            { id: 'amount_q2', label: 'Q2', type: 'money', pivot: { cell: 'amount', key: 2 } },
            { id: 'amount_q3', label: 'Q3', type: 'money', pivot: { cell: 'amount', key: 3 } },
        ]
        const layout = tableLines({
            columns,
            width: 390,
            measures: uniform(columns, 110),
            layouts: [
                {
                    width: 'mobile',
                    tracks: 2,
                    cells: [
                        { col: 'account', line: 1 },
                        { col: 'amount', line: 1 },
                    ],
                },
            ],
        })
        const cell = (col: string) => ({ kind: 'cell', col, justify: 'end' })
        expect(shapeOf(layout)).toEqual({
            kind: 'blocks',
            root: {
                kind: 'stack',
                parts: [
                    {
                        kind: 'side',
                        of: 2,
                        parts: [
                            {
                                block: { kind: 'cell', col: 'account', justify: 'start' },
                                tracks: 1,
                            },
                            { block: cell('amount_q1'), tracks: 1 },
                        ],
                    },
                    {
                        kind: 'side',
                        of: 2,
                        parts: [
                            { block: cell('amount_q2'), tracks: 1 },
                            { block: cell('amount_q3'), tracks: 1 },
                        ],
                    },
                ],
            },
        })
    })
    it('gives an empty part as many lines as it stands for', () => {
        const layout = tableLines({
            columns: COLS,
            width: 390,
            measures: COLS_110,
            layouts: [
                {
                    width: 'mobile',
                    tracks: 2,
                    cells: [
                        { col: 'account', line: 1, lines: 3 },
                        { col: 'debit', line: 1 },
                    ],
                },
            ],
        })
        expect(shapeOf(layout)).toEqual({
            kind: 'blocks',
            root: {
                kind: 'side',
                of: 2,
                parts: [
                    { block: { kind: 'cell', col: 'account', justify: 'start' }, tracks: 1 },
                    {
                        block: {
                            kind: 'stack',
                            parts: [
                                { kind: 'cell', col: 'debit', justify: 'end' },
                                { kind: 'gap', lines: 2 },
                            ],
                        },
                        tracks: 1,
                    },
                ],
            },
        })
    })
    it('keeps the place and the lines of a placed column the run does not show', () => {
        const shown = COLS.filter(c => c.id !== 'number')
        // Three columns at 110 need 330 on one line.
        const layout = tableLines({
            columns: shown,
            width: 300,
            measures: uniform(shown, 110),
            layouts: [
                {
                    width: 'mobile',
                    tracks: 2,
                    cells: [
                        { col: 'account', line: 1, span: 2 },
                        { col: 'number', line: 2, lines: 2 },
                        { col: 'debit', line: 2 },
                        { col: 'credit', line: 3, start: 2 },
                    ],
                },
            ],
        })
        expect(shapeOf(layout)).toEqual({
            kind: 'blocks',
            root: {
                kind: 'stack',
                parts: [
                    { kind: 'cell', col: 'account', justify: 'start' },
                    {
                        kind: 'side',
                        of: 2,
                        parts: [
                            { block: { kind: 'gap', lines: 2 }, tracks: 1 },
                            {
                                block: {
                                    kind: 'stack',
                                    parts: [
                                        { kind: 'cell', col: 'debit', justify: 'end' },
                                        { kind: 'cell', col: 'credit', justify: 'end' },
                                    ],
                                },
                                tracks: 1,
                            },
                        ],
                    },
                ],
            },
        })
    })
    it('starts the label column at the left in an arrangement, whatever its type', () => {
        const columns: ResultColumn[] = [
            { id: 'number', label: 'No.', type: 'number' },
            { id: 'debit', label: 'Debit', type: 'money', currency: 'USD' },
        ]
        // One line needs 220, so the two-line arrangement is used at 200.
        const layout = tableLines({
            columns,
            width: 200,
            measures: uniform(columns, 110),
            layouts: [
                {
                    width: 'mobile',
                    tracks: 1,
                    cells: [
                        { col: 'number', line: 1 },
                        { col: 'debit', line: 2 },
                    ],
                },
            ],
        })
        expect(shapeOf(layout)).toEqual({
            kind: 'blocks',
            root: {
                kind: 'stack',
                parts: [
                    { kind: 'cell', col: 'number', justify: 'start' },
                    { kind: 'cell', col: 'debit', justify: 'end' },
                ],
            },
        })
    })
    const PIVOTS: ResultColumn[] = [
        { id: 'account', label: 'Account', type: 'text' },
        { id: 'number', label: 'No.', type: 'text' },
        { id: 'amount_q1', label: 'Q1', type: 'money', pivot: { cell: 'amount', key: 1 } },
        { id: 'amount_q2', label: 'Q2', type: 'money', pivot: { cell: 'amount', key: 2 } },
        { id: 'amount_q3', label: 'Q3', type: 'money', pivot: { cell: 'amount', key: 3 } },
    ]
    // Five columns at 110 need 550 on one line; four side by side fit 500.
    const PIVOTS_110 = uniform(PIVOTS, 110)
    const pivotLines = (cells: DisplayCell[], tracks: number) =>
        tableLines({
            columns: PIVOTS,
            width: 500,
            measures: PIVOTS_110,
            layouts: [{ width: 'mobile', tracks, cells }],
        })
    const pivotShape = (cells: DisplayCell[], tracks: number) => shapeOf(pivotLines(cells, tracks))
    const pivotCell = (col: string) => ({ kind: 'cell', col, justify: 'end' })
    it('gives each pivot column the span and lines of its entry', () => {
        expect(
            pivotShape(
                [
                    { col: 'account', line: 1, lines: 2 },
                    { col: 'amount', line: 1, lines: 2 },
                ],
                4
            )
        ).toEqual({
            kind: 'blocks',
            root: {
                kind: 'side',
                of: 4,
                parts: [
                    { block: { kind: 'cell', col: 'account', justify: 'start' }, tracks: 1 },
                    { block: pivotCell('amount_q1'), tracks: 1 },
                    { block: pivotCell('amount_q2'), tracks: 1 },
                    { block: pivotCell('amount_q3'), tracks: 1 },
                ],
            },
        })
        expect(
            pivotShape(
                [
                    { col: 'account', line: 1, span: 4 },
                    { col: 'amount', line: 2, span: 2 },
                ],
                4
            )
        ).toEqual({
            kind: 'blocks',
            root: {
                kind: 'stack',
                parts: [
                    { kind: 'cell', col: 'account', justify: 'start' },
                    {
                        kind: 'side',
                        of: 4,
                        parts: [
                            { block: pivotCell('amount_q1'), tracks: 2 },
                            { block: pivotCell('amount_q2'), tracks: 2 },
                        ],
                    },
                    {
                        kind: 'side',
                        of: 4,
                        parts: [
                            { block: pivotCell('amount_q3'), tracks: 2 },
                            { block: { kind: 'gap', lines: 1 }, tracks: 2 },
                        ],
                    },
                ],
            },
        })
    })
    it('lays out automatically, and says so, when the pivot columns collide with a cell', () => {
        const auto = tableLines({ columns: PIVOTS, width: 500, layouts: [], measures: PIVOTS_110 })
        expect(auto.kind).toBe('blocks')
        expect(
            pivotLines(
                [
                    { col: 'account', line: 1 },
                    { col: 'amount', line: 1 },
                    { col: 'number', line: 2 },
                ],
                2
            )
        ).toEqual({ ...auto, notice: PIVOT_NOTICE })
    })
    it('lays out automatically, and says so, when an arrangement cannot be placed', () => {
        // Σmin 404 > 300: one line does not fit.
        const auto = tableLines({ columns: COLS, width: 300, layouts: [], measures: COLS_MEASURES })
        const overlap: DisplayLayout = {
            width: 'mobile',
            tracks: 2,
            cells: [
                { col: 'account', line: 1, span: 2 },
                { col: 'number', line: 1 },
            ],
        }
        const result = tableLines({
            columns: COLS,
            width: 300,
            layouts: [overlap],
            measures: COLS_MEASURES,
        })
        // The literal: a missing export reads as undefined and would pass a toEqual.
        expect(result).toMatchObject({
            notice: "This width's arrangement cannot be laid out, so the rows are laid out automatically here.",
        })
        expect(result).toStrictEqual({ ...auto, notice: ARRANGEMENT_NOTICE })
    })
    it('a row with no figures draws only its label', () => {
        const layout = tableLines({
            columns: COLS,
            width: 360,
            layouts: [],
            measures: WIDE_LABEL_MEASURES,
        })
        if (layout.kind !== 'blocks') throw new Error('expected lines')
        const row = { id: 'g', kind: 'group' as const, depth: 0, label: 'Assets', cells: {} }
        expect(
            rowLayout({ row, cells: {}, isToggle: true, isOpen: false }, layout, 'account', 0)
        ).toEqual({
            kind: 'blocks',
            root: { kind: 'cell', col: 'account', justify: 'start' },
            indent: layout.indent,
        })
        const full = { number: 'A1', debit: 5, credit: 0 }
        expect(
            rowLayout({ row, cells: full, isToggle: false, isOpen: false }, layout, 'account', 0)
        ).toBe(layout)
    })
    // E2: an opened row with no No. cell (blank on detail rows) keeps the phone lines,
    // so No. is an empty slot under its heading and the amounts stay in their tracks.
    it('keeps the wrapped lines of a detail row whose No. is blank', () => {
        const layout = tableLines({
            columns: COLS,
            width: 360,
            layouts: [],
            measures: WIDE_LABEL_MEASURES,
        })
        expect(layout.kind).toBe('blocks')
        const row = { id: 'd:1', kind: 'detail' as const, depth: 1, label: 'Sale', cells: {} }
        const cells = { debit: 250, credit: null }
        expect(
            rowLayout({ row, cells, isToggle: false, isOpen: false }, layout, 'account', 0)
        ).toBe(layout)
    })
    it('starts an arrangement from the columns', () => {
        expect(startingLayout('mobile', [{ id: 'a' }, { id: 'b' }, { id: 'c' }])).toEqual({
            width: 'mobile',
            tracks: 3,
            cells: [
                { col: 'a', line: 1, span: 3 },
                { col: 'b', line: 2 },
                { col: 'c', line: 2 },
            ],
        })
    })
})

// Nine columns: at 110 each, one line needs 990.
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
const DESKTOP_LAYOUT = {
    width: 'desktop' as const,
    tracks: 2,
    cells: [
        { col: 'customer', line: 1, span: 2 },
        { col: 'amount', line: 2 },
        { col: 'balance', line: 2 },
    ],
}
const TABLET_LAYOUT = {
    width: 'tablet' as const,
    tracks: 3,
    cells: [
        { col: 'customer', line: 1, span: 3 },
        { col: 'amount', line: 2 },
        { col: 'cost', line: 2 },
        { col: 'balance', line: 2 },
    ],
}
const MOBILE_LAYOUT = {
    width: 'mobile' as const,
    tracks: 2,
    cells: [
        { col: 'customer', line: 1, span: 2 },
        { col: 'balance', line: 2, start: 2 },
    ],
}
const label = { kind: 'cell', col: 'customer', justify: 'start' } as const
const money = (col: string) => ({
    block: { kind: 'cell', col, justify: 'end' } as const,
    tracks: 1,
})
const DESKTOP_LINES = {
    kind: 'blocks',
    root: {
        kind: 'stack',
        parts: [label, { kind: 'side', of: 2, parts: [money('amount'), money('balance')] }],
    },
}
const TABLET_LINES = {
    kind: 'blocks',
    root: {
        kind: 'stack',
        parts: [
            label,
            { kind: 'side', of: 3, parts: [money('amount'), money('cost'), money('balance')] },
        ],
    },
}
const MOBILE_LINES = {
    kind: 'blocks',
    root: {
        kind: 'stack',
        parts: [
            label,
            {
                kind: 'side',
                of: 2,
                parts: [{ block: { kind: 'gap', lines: 1 }, tracks: 1 }, money('balance')],
            },
        ],
    },
}

describe('the layout chosen by fit', () => {
    // WIDE at min 110 each: one line needs 990.
    const measures = uniform(WIDE, 110)
    const at = (width: number, layouts: DisplayLayout[] = []) =>
        tableLines({ columns: WIDE, width, layouts, measures })

    it('draws one line wherever it fits, an arrangement or not', () => {
        expect(at(1000, [DESKTOP_LAYOUT, TABLET_LAYOUT, MOBILE_LAYOUT]).kind).toBe('one-line')
    })

    it('uses the widest arrangement whose lines fit, then the narrower ones', () => {
        // The fixtures carry no widths and no indent, so they compare with the shape.
        expect(shapeOf(at(900, [DESKTOP_LAYOUT, TABLET_LAYOUT]))).toEqual(DESKTOP_LINES)
        expect(shapeOf(at(900, [MOBILE_LAYOUT, TABLET_LAYOUT]))).toEqual(TABLET_LINES)
        expect(shapeOf(at(400, [MOBILE_LAYOUT]))).toEqual(MOBILE_LINES)
        // At 300 the tablet's second line needs 330 (3 × 110) and 296 is left under
        // the indent, so it falls through to the mobile lines, which fit.
        expect(shapeOf(at(300, [TABLET_LAYOUT, MOBILE_LAYOUT]))).toEqual(MOBILE_LINES)
    })

    it('never gives a part a negative width at a width below the indent', () => {
        const layout = tableLines({
            columns: COLS,
            width: 40,
            layouts: [],
            levels: { groups: 3, nests: 0 },
            measures: uniform(COLS, 110),
        })
        if (layout.kind !== 'blocks') throw new Error('expected lines')
        expect(layout.indent).toBeGreaterThan(40)
        const widths = (block: Block): number[] => {
            if (block.kind === 'stack') return block.parts.flatMap(widths)
            if (block.kind !== 'side') return []
            return block.parts.flatMap(part => [part.width ?? 0, ...widths(part.block)])
        }
        expect(Math.min(...widths(layout.root))).toBeGreaterThanOrEqual(0)
    })

    it('falls to the automatic lines when no arrangement fits', () => {
        // The label's min (110) is wider than the view: no arrangement fits.
        const auto = at(100, [MOBILE_LAYOUT])
        expect(auto.kind).toBe('blocks')
        expect(shapeOf(auto)).not.toEqual(MOBILE_LINES)
    })

    it('gives a side block its width by content, a span its share of the spare room', () => {
        const layout = tableLines({
            columns: COLS,
            width: 360,
            layouts: [
                {
                    width: 'mobile',
                    tracks: 3,
                    cells: [
                        { col: 'account', line: 1, span: 2 },
                        { col: 'number', line: 1 },
                        { col: 'debit', line: 2 },
                        { col: 'credit', line: 2, start: 3 },
                    ],
                },
            ],
            measures: {
                account: m(100, 100, 1),
                number: m(60, 60, 1),
                debit: m(110, 110, 0),
                credit: m(110, 110, 0),
            },
        })
        if (layout.kind !== 'blocks' || layout.root.kind !== 'stack')
            throw new Error('expected lines')
        const [first, second] = layout.root.parts
        if (first?.kind !== 'side' || second?.kind !== 'side')
            throw new Error('expected side lines')
        // One line needs 380, so the arrangement is used. Line 1: 200 spare by
        // tracks 2 : 1 → 133 and 66, the remainder 1 to the first part.
        expect(first.parts.map(p => p.width)).toEqual([234, 126])
        // The second line is fitted under the indent (a flat report: 16 − 12 = 4):
        // 356 − 220 = 136 spare by tracks 1 : 1 : 1, the empty track included.
        expect(layout.indent).toBe(4)
        expect(second.parts.map(p => p.width)).toEqual([156, 45, 155])
    })

    it('packs the automatic lines by content, the label sharing line 1', () => {
        const layout = tableLines({
            columns: COLS,
            width: 300,
            layouts: [],
            measures: {
                account: m(120, 120, 1),
                number: m(60, 60, 1),
                debit: m(110, 110, 0),
                credit: m(110, 110, 0),
            },
        })
        expect(shapeOf(layout)).toEqual({
            kind: 'blocks',
            root: {
                kind: 'stack',
                parts: [
                    {
                        kind: 'side',
                        of: 3,
                        parts: [
                            {
                                block: { kind: 'cell', col: 'account', justify: 'start' },
                                tracks: 1,
                            },
                            { block: { kind: 'cell', col: 'number', justify: 'start' }, tracks: 1 },
                            { block: { kind: 'cell', col: 'debit', justify: 'end' }, tracks: 1 },
                        ],
                    },
                    {
                        kind: 'side',
                        of: 1,
                        parts: [
                            { block: { kind: 'cell', col: 'credit', justify: 'end' }, tracks: 1 },
                        ],
                    },
                ],
            },
        })
    })

    it('clamps a column wider than its line, and never draws past the width', () => {
        const layout = tableLines({
            columns: COLS,
            width: 100,
            layouts: [],
            measures: {
                account: m(150, 150, 1),
                number: m(60, 60, 1),
                debit: m(110, 110, 0),
                credit: m(110, 110, 0),
            },
        })
        if (layout.kind !== 'blocks' || layout.root.kind !== 'stack')
            throw new Error('expected lines')
        for (const [i, line] of layout.root.parts.entries()) {
            if (line.kind !== 'side') throw new Error('expected side lines')
            const room = i === 0 ? 100 : 100 - layout.indent
            expect(line.parts.reduce((s, p) => s + (p.width ?? 0), 0)).toBe(room)
        }
    })

    it('starts the continuation lines under a detail row’s label text', () => {
        expect(continuationIndent({ groups: 0, nests: 0 })).toBe(4)
        expect(continuationIndent({ groups: 1, nests: 0 })).toBe(20)
        expect(continuationIndent({ groups: 1, nests: 1 })).toBe(36)
    })
})

// R14: a row that is not a detail row draws only the lines it has a value on.
describe('figure-only rows in a wrapped layout', () => {
    const PHONE = {
        width: 'mobile' as const,
        tracks: 3,
        cells: [
            { col: 'customer', line: 1, span: 3 },
            { col: 'status', line: 2 },
            { col: 'ref', line: 2 },
            { col: 'team', line: 3, span: 3 },
            { col: 'amount', line: 4 },
            { col: 'cost', line: 4 },
            { col: 'balance', line: 4 },
        ],
    }
    const COLUMNS = WIDE.filter(c => c.id !== 'note' && c.id !== 'other')
    const layout = tableLines({
        columns: COLUMNS,
        width: 390,
        layouts: [PHONE],
        measures: uniform(COLUMNS, 110),
    })
    const item = (kind: ResultRow['kind'], cells: Record<string, CellValue>) => ({
        row: { id: `${kind}:1`, kind, depth: 1, label: 'Before the period', cells },
        cells,
        isToggle: false,
        isOpen: false,
    })

    it('lets the label take the empty tracks of its line', () => {
        const cells = { amount: 900, cost: 300, balance: null }
        const row = rowLayout(item('subtotal', cells), layout, 'customer', 999)
        if (row.kind !== 'blocks' || row.root.kind !== 'stack') throw new Error('expected lines')
        // Line 1 is the label alone, across the row; the blank balance keeps its track.
        expect(row.root.parts[0]).toEqual(label)
        expect(shapeOf({ ...row, root: row.root.parts[1] as Block })).toMatchObject({
            root: {
                kind: 'side',
                of: 3,
                parts: [{ tracks: 1 }, { tracks: 1 }, { block: { kind: 'gap' } }],
            },
        })
    })

    it('puts an opening row’s label beside its figure when the empty tracks hold it (H7)', () => {
        const row = rowLayout(item('opening', { balance: 1200 }), layout, 'customer', 150)
        if (row.kind !== 'blocks' || row.root.kind !== 'side') throw new Error('expected one line')
        const [labelPart, figure] = row.root.parts
        expect(labelPart?.block).toEqual(label)
        expect(labelPart?.tracks).toBe(2)
        expect(figure?.block).toEqual(money('balance').block)
        // The two empty tracks and the indent: the line keeps the row's width.
        if (layout.kind !== 'blocks' || layout.root.kind !== 'stack')
            throw new Error('expected lines')
        const line = layout.root.parts.at(-1) as Extract<Block, { kind: 'side' }>
        expect(labelPart?.width).toBe(
            (line.parts[0]?.width ?? 0) + (line.parts[1]?.width ?? 0) + layout.indent
        )
    })

    it('keeps the label on its own line when the empty tracks are too narrow', () => {
        const row = rowLayout(item('opening', { balance: 1200 }), layout, 'customer', 10_000)
        if (row.kind !== 'blocks') throw new Error('expected blocks')
        expect(row.root.kind).toBe('stack')
    })

    it('keeps every line of a detail row, and of any row with a value in every column', () => {
        expect(rowLayout(item('detail', { balance: 1200 }), layout, 'customer', 0)).toBe(layout)
        const full = { status: 'open', ref: 'A1', team: 't1', amount: 1, cost: 2, balance: 3 }
        expect(rowLayout(item('total', full), layout, 'customer', 0)).toBe(layout)
    })
})

// F10 over a pivot: the last group level holds the pivot's cells, so its groups
// never open. They draw no caret and no row comes under them, so the label min
// is set by the deepest label start that the report can really draw.
describe('the label min and the deepest label start', () => {
    // The label's min is for its text, past the deepest row's indent.
    it('the label min grows with the deepest label start', () => {
        const flatLevels = { groups: 0, nests: 0 }
        const groupedLevels = { groups: 1, nests: 0 }
        expect(labelMinOf(flatLevels)).toBe(labelChrome(flatLevels) + TEXT_MIN_WIDTH)
        expect(labelMinOf(groupedLevels)).toBe(labelChrome(groupedLevels) + TEXT_MIN_WIDTH)
        expect(labelMinOf(groupedLevels) - labelMinOf(flatLevels)).toBe(CARET_ROOM)
    })

    const PIVOT = { path: [{ field: 'month' }], cell: 'amount' }
    const columns = [{ path: [{ field: 'team' }] }]

    it('marks a spec with a pivot', () => {
        expect(rowLevelsOf({ groups: [{}], columns, pivot: PIVOT })).toEqual({
            groups: 1,
            nests: 0,
            pivot: true,
        })
        expect(rowLevelsOf({ groups: [{}], columns })).toEqual({ groups: 1, nests: 0 })
    })

    it('counts the pivot level without a caret and with nothing under it', () => {
        // One level: the pivot's rows sit at the left edge, like a flat report.
        expect(deepestLabelStart({ groups: 1, nests: 0, pivot: true })).toBe(0)
        expect(deepestLabelStart({ groups: 1, nests: 0 })).toBe(18)
        // Two levels: the outer group's caret (18) passes the pivot rows' indent (16).
        expect(deepestLabelStart({ groups: 2, nests: 0, pivot: true })).toBe(18)
        expect(deepestLabelStart({ groups: 2, nests: 0 })).toBe(34)
        expect(deepestLabelStart({ groups: 3, nests: 0, pivot: true })).toBe(34)
        expect(deepestLabelStart({ groups: 0, nests: 0 })).toBe(0)
    })

    it('gives a one-level pivot a label min 18 px less than a grouped report’s', () => {
        const minOf = (levels: RowLevels) => labelMinOf(levels)
        expect(minOf({ groups: 1, nests: 0 }) - minOf({ groups: 1, nests: 0, pivot: true })).toBe(
            CARET_ROOM
        )
    })
})

// F10: a group row's label is not held to the label column's width when the
// cells after it are empty, as its blocks lines draw only its label.
describe('the label span of a one-line row', () => {
    const layout = tableLines({
        columns: COLS,
        width: 1280,
        layouts: [],
        levels: { groups: 1, nests: 0 },
    })
    const others = ['number', 'debit', 'credit']
    const item = (kind: ResultRow['kind'], cells: Record<string, CellValue>) => ({
        row: { id: `${kind}:1`, kind, depth: 0, label: '1000 Cash and cash equivalents', cells },
        cells,
        isToggle: kind === 'group',
        isOpen: kind === 'group',
    })

    it('spans an open group row (no cells) across every column', () => {
        expect(oneLineLabelSpan(item('group', {}), others)).toBe(3)
    })

    it('spans a closed group row up to its first value, and blank cells count as empty', () => {
        expect(
            oneLineLabelSpan(item('group', { number: null, debit: 12, credit: 3 }), others)
        ).toBe(1)
        expect(oneLineLabelSpan(item('group', { debit: 12 }), others)).toBe(1)
        expect(oneLineLabelSpan(item('group', { number: 'A', debit: 12 }), others)).toBe(0)
    })

    it('spans every non-detail row up to its first value, and no detail row', () => {
        expect(oneLineLabelSpan(item('detail', {}), others)).toBe(0)
        expect(oneLineLabelSpan(item('subtotal', { credit: 1 }), others)).toBe(2)
        expect(oneLineLabelSpan(item('opening', { credit: 1 }), others)).toBe(2)
        expect(oneLineLabelSpan(item('total', { debit: 5, credit: 1 }), others)).toBe(1)
        expect(oneLineLabelSpan(item('computed', {}), others)).toBe(3)
    })

    it('widens the label box by the columns it spans, so later cells keep their place', () => {
        if (layout.kind !== 'one-line') throw new Error('expected one line')
        const { widths } = layout
        expect(oneLineLabelWidth(layout, 'account', [])).toBe(widths.account)
        expect(oneLineLabelWidth(layout, 'account', ['number'])).toBe(
            widths.account + widths.number
        )
        expect(oneLineLabelWidth(layout, 'account', others)).toBe(
            widths.account + widths.number + widths.debit + widths.credit
        )
        expect(oneLineLabelWidth(layout, undefined, others)).toBeUndefined()
        expect(
            oneLineLabelWidth({ kind: 'blocks', root: label, indent: 0 }, 'account', [])
        ).toBeUndefined()
    })
})

describe('the label span of a side block', () => {
    const labelCell: Block = { kind: 'cell', col: 'account', justify: 'start' }
    const debitCell: Block = { kind: 'cell', col: 'debit', justify: 'end' }
    const gap: Block = { kind: 'gap', lines: 1 }

    it('gives the label the empty parts right of it: their tracks and widths add up', () => {
        const side: Block = {
            kind: 'side',
            of: 6,
            parts: [
                { block: labelCell, tracks: 2, width: 100 },
                { block: gap, tracks: 1, width: 40 },
                { block: gap, tracks: 1, width: 30 },
                { block: debitCell, tracks: 2, width: 80 },
            ],
        }
        if (side.kind !== 'side') throw new Error('expected a side block')
        expect(labelSpans(side, 'account')).toEqual({
            kind: 'side',
            of: 6,
            parts: [
                { block: labelCell, tracks: 4, width: 170 },
                { block: debitCell, tracks: 2, width: 80 },
            ],
        })
    })

    it('keeps a gap left of the label, and a part with no width leaves the sum unknown', () => {
        const side = {
            kind: 'side' as const,
            of: 4,
            parts: [
                { block: gap, tracks: 1, width: 20 },
                { block: labelCell, tracks: 1, width: 60 },
                { block: gap, tracks: 2 },
            ],
        }
        expect(labelSpans(side, 'account')).toEqual({
            kind: 'side',
            of: 4,
            parts: [
                { block: gap, tracks: 1, width: 20 },
                { block: labelCell, tracks: 3, width: undefined },
            ],
        })
    })

    it('is the label block when the label takes every part', () => {
        const side = {
            kind: 'side' as const,
            of: 3,
            parts: [
                { block: labelCell, tracks: 1, width: 60 },
                { block: gap, tracks: 2, width: 90 },
            ],
        }
        expect(labelSpans(side, 'account')).toEqual(labelCell)
    })

    it('leaves a gap after another cell', () => {
        const side = {
            kind: 'side' as const,
            of: 3,
            parts: [
                { block: debitCell, tracks: 1, width: 60 },
                { block: gap, tracks: 2, width: 90 },
            ],
        }
        expect(labelSpans(side, 'account')).toEqual(side)
    })
})

describe('column fit in the fit', () => {
    const FIT_MEASURES: ColumnMeasures = {
        account: m(120, 180, 1),
        number: m(64, 64, 1),
        debit: m(110, 110, 0),
        credit: m(110, 110, 0),
    }
    // Mins: 120 + 64 + 110 + 110 = 404.
    const fitAt = (width: number, extra: Partial<Parameters<typeof fitTable>[0]> = {}) =>
        fitTable({ columns: COLS, width, layouts: [], measures: FIT_MEASURES, ...extra })
    const TABLET: DisplayLayout = {
        width: 'tablet',
        tracks: 2,
        cells: [
            { col: 'account', line: 1, span: 2 },
            { col: 'number', line: 2 },
            { col: 'debit', line: 2 },
            { col: 'credit', line: 3 },
        ],
    }

    it('stops a grower at its max and shares the rest among the others', () => {
        const capped: ColumnMeasure = { min: 80, natural: 120, grow: 1, max: 150 }
        expect(shareWidth([m(100, 100, 1), capped], 600)).toEqual([450, 150])
        expect(
            shareWidth(
                [m(100, 100, 1), { min: 100, natural: 100, grow: 1, max: 120 }, m(100, 100, 1)],
                600
            )
        ).toEqual([240, 120, 240])
    })

    it('resolves a % at the line width', () => {
        expect(atWidth({ natural: 100, min: 50, grow: 1, pct: { width: 0.25 } }, 800)).toEqual({
            natural: 200,
            min: 200,
            max: 200,
            grow: 1,
        })
        expect(
            atWidth({ natural: 300, min: 50, grow: 1, pct: { min: 0.1, max: 0.2 } }, 1000)
        ).toEqual({
            natural: 200,
            min: 100,
            max: 200,
            grow: 1,
        })
        const plain = m(50, 100, 1)
        expect(atWidth(plain, 1000)).toBe(plain)
    })

    it('a % width is a part of the line', () => {
        const measures = { ...FIT_MEASURES, number: { ...m(64, 64, 1), pct: { width: 0.25 } } }
        expect(fitAt(800, { measures }).layout).toEqual({
            kind: 'one-line',
            widths: { account: 380, number: 200, debit: 110, credit: 110 },
        })
    })

    it('draws one line only from the one-line width', () => {
        expect(fitAt(500, { oneLine: { minWidth: '600px' } }).source).toBe('auto')
        expect(fitAt(600, { oneLine: { minWidth: '600px' } }).source).toBe('one-line')
        expect(fitAt(500).source).toBe('one-line')
    })

    it('never one line under the gate, however wide the columns fit', () => {
        const wide = fitAt(1280, { oneLine: { minWidth: '4000px' } })
        expect(wide.source).toBe('auto')
        expect(wide.layout.kind).toBe('blocks')
    })

    it('uses an arrangement only from its minWidth, and says nothing when it skips it', () => {
        expect(fitAt(380, { layouts: [{ ...TABLET, minWidth: '350px' }] }).source).toBe('tablet')
        const skipped = fitAt(380, { layouts: [{ ...TABLET, minWidth: '390px' }] })
        expect(skipped.source).toBe('auto')
        expect(skipped.layout.kind === 'blocks' && skipped.layout.notice).toBeFalsy()
    })

    it('carries the lines a wrapping column may take, on every row', () => {
        const measures = { ...FIT_MEASURES, account: { ...m(120, 180, 1), lines: 2 } }
        expect(fitAt(1280, { measures }).layout).toMatchObject({
            kind: 'one-line',
            lines: { account: 2 },
        })
        const auto = fitAt(300, { measures }).layout
        expect(auto).toMatchObject({ kind: 'blocks', lines: { account: 2 } })
        const group: VisibleRow = {
            row: { id: 'group:A', kind: 'group', depth: 0, label: 'A', cells: {} },
            cells: {},
            isToggle: true,
            isOpen: false,
        }
        expect(rowLayout(group, auto, 'account', 0)).toMatchObject({ lines: { account: 2 } })
        expect('lines' in fitAt(1280).layout).toBe(false)
    })

    it('says which rule laid the rows out', () => {
        expect(fitAt(1280).source).toBe('one-line')
        expect(fitAt(300).source).toBe('auto')
        expect(
            tableLines({ columns: COLS, width: 1280, layouts: [], measures: FIT_MEASURES })
        ).toEqual(fitAt(1280).layout)
    })
})

describe('an authored layout gives up indent before it is given up (G9)', () => {
    const columns: ResultColumn[] = [
        { id: 'account', label: 'Account', type: 'text' },
        { id: 'current', label: 'Current', type: 'money', currency: 'USD' },
        { id: 'prior', label: 'Prior year', type: 'money', currency: 'USD' },
        { id: 'change', label: 'Change', type: 'money', currency: 'USD' },
        { id: 'pct', label: '% change', type: 'percent' },
    ]
    // The phone layout leaves % change out: the label, then three figures.
    const mobile: DisplayLayout = {
        width: 'mobile',
        tracks: 3,
        cells: [
            { col: 'account', line: 1, span: 3 },
            { col: 'current', line: 2 },
            { col: 'prior', line: 2 },
            { col: 'change', line: 2 },
        ],
    }
    // Three figures of 105 need 315 on line 2.
    const measures: ColumnMeasures = {
        account: m(100, 100, 1),
        current: m(105, 105, 0),
        prior: m(105, 105, 0),
        change: m(105, 105, 0),
        pct: m(105, 105, 0),
    }
    const levels: RowLevels = { groups: 4, nests: 0 }
    const fitAt = (width: number, layouts: DisplayLayout[] = [mobile]) =>
        fitTable({ columns, width, layouts, levels, measures })

    it('fits at the largest smaller indent when the full indent leaves too little room', () => {
        // 378 − 68 (four levels) = 310 < 315; 378 − 52 (three levels) = 326.
        const { layout, source } = fitAt(378)
        expect(source).toBe('mobile')
        if (layout.kind !== 'blocks') throw new Error('expected lines')
        expect(layout.indent).toBe(continuationIndent({ groups: 3, nests: 0 }))
        expect(layout.indent).toBe(52)
    })

    it('keeps the full indent where the lines fit at it', () => {
        const { layout, source } = fitAt(400)
        expect(source).toBe('mobile')
        if (layout.kind !== 'blocks') throw new Error('expected lines')
        expect(layout.indent).toBe(continuationIndent(levels))
    })

    it('tries indent 0 last', () => {
        // 318 − 4 (no level) = 314 < 315; 318 − 0 = 318.
        const { layout, source } = fitAt(318)
        expect(source).toBe('mobile')
        if (layout.kind !== 'blocks') throw new Error('expected lines')
        expect(layout.indent).toBe(0)
    })

    it('falls to automatic lines when the layout does not fit even at indent 0', () => {
        expect(fitAt(300).source).toBe('auto')
    })

    it('still does not try a layout below its minWidth', () => {
        expect(fitAt(378, [{ ...mobile, minWidth: '400px' }]).source).toBe('auto')
    })
})
