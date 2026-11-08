import { describe, expect, it } from 'vitest'
import { z } from 'zod'
import table from '../fixtures/display-cells.json'
import { type DisplayCell, type DisplayLayout, displayCellSchema } from '../spec'
import {
    fitOneLine,
    labelFloor,
    oneLineLabelSpan,
    oneLineLabelWidth,
    PINWHEEL_MESSAGE,
    PIVOT_NOTICE,
    placeCells,
    rowLayout,
    rowLevelsOf,
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
    // Keep equal to the count in the Go test.
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

describe('the table layout per width', () => {
    it('draws one line with its natural widths wherever the columns fit', () => {
        const natural = { account: 220, number: 128, debit: 128, credit: 128 }
        expect(
            tableLines({ columns: COLS, width: 1280, breakpoint: 'desktop', layouts: [] })
        ).toEqual({ kind: 'one-line', widths: natural })
        expect(
            tableLines({ columns: COLS, width: 900, breakpoint: 'tablet', layouts: [] })
        ).toEqual({ kind: 'one-line', widths: natural })
    })
    it('wraps a phone row: the label across line 1, the amounts in aligned tracks below', () => {
        expect(
            tableLines({
                columns: COLS,
                width: 360,
                breakpoint: 'mobile',
                layouts: [],
            })
        ).toEqual({
            kind: 'blocks',
            root: {
                kind: 'stack',
                parts: [
                    { kind: 'cell', col: 'account', justify: 'start' },
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
    it('wraps the amounts onto more lines when the tracks run out, a short last line keeping its tracks', () => {
        expect(
            tableLines({
                columns: COLS,
                width: 300,
                breakpoint: 'tablet',
                layouts: [],
            })
        ).toEqual({
            kind: 'blocks',
            root: {
                kind: 'stack',
                parts: [
                    { kind: 'cell', col: 'account', justify: 'start' },
                    {
                        kind: 'side',
                        of: 2,
                        parts: [
                            { block: { kind: 'cell', col: 'number', justify: 'start' }, tracks: 1 },
                            { block: { kind: 'cell', col: 'debit', justify: 'end' }, tracks: 1 },
                        ],
                    },
                    {
                        kind: 'side',
                        of: 2,
                        parts: [
                            { block: { kind: 'cell', col: 'credit', justify: 'end' }, tracks: 1 },
                            { block: { kind: 'gap', lines: 1 }, tracks: 1 },
                        ],
                    },
                ],
            },
        })
    })
    it('cuts an arrangement with a mid-row cell spanning two lines into side blocks', () => {
        const layout = tableLines({
            columns: COLS,
            width: 400,
            breakpoint: 'mobile',
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
        expect(layout).toEqual({
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
            breakpoint: 'mobile',
            layouts: [{ width: 'mobile', tracks: 2, cells: [{ col: 'debit', line: 1, span: 2 }] }],
        })
        expect(layout).toEqual({
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
    it('uses the arrangement for its own width only, and lays out a refused one automatically', () => {
        const mobile = {
            width: 'mobile' as const,
            tracks: 2,
            cells: [{ col: 'debit', line: 1, span: 2 }],
        }
        expect(
            tableLines({
                columns: COLS,
                width: 1280,
                breakpoint: 'desktop',
                layouts: [mobile],
            })
        ).toMatchObject({ kind: 'one-line' })
        const auto = tableLines({
            columns: COLS,
            width: 360,
            breakpoint: 'mobile',
            layouts: [],
        })
        const refused = { ...mobile, cells: [{ col: 'debit', line: 9 }] }
        expect(
            tableLines({
                columns: COLS,
                width: 360,
                breakpoint: 'mobile',
                layouts: [refused],
            })
        ).toEqual(auto)
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
            breakpoint: 'mobile',
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
        expect(layout).toEqual({
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
            breakpoint: 'mobile',
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
        expect(layout).toEqual({
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
        const layout = tableLines({
            columns: shown,
            width: 390,
            breakpoint: 'mobile',
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
        expect(layout).toEqual({
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
        const layout = tableLines({
            columns,
            width: 390,
            breakpoint: 'mobile',
            layouts: [
                {
                    width: 'mobile',
                    tracks: 2,
                    cells: [
                        { col: 'number', line: 1 },
                        { col: 'debit', line: 1 },
                    ],
                },
            ],
        })
        expect(layout).toEqual({
            kind: 'blocks',
            root: {
                kind: 'side',
                of: 2,
                parts: [
                    { block: { kind: 'cell', col: 'number', justify: 'start' }, tracks: 1 },
                    { block: { kind: 'cell', col: 'debit', justify: 'end' }, tracks: 1 },
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
    const pivotLines = (cells: DisplayCell[], tracks: number) =>
        tableLines({
            columns: PIVOTS,
            width: 390,
            breakpoint: 'mobile',
            layouts: [{ width: 'mobile', tracks, cells }],
        })
    const pivotCell = (col: string) => ({ kind: 'cell', col, justify: 'end' })
    it('gives each pivot column the span and lines of its entry', () => {
        expect(
            pivotLines(
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
            pivotLines(
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
        const auto = tableLines({
            columns: PIVOTS,
            width: 390,
            breakpoint: 'mobile',
            layouts: [],
        })
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
    it('a row with no figures draws only its label', () => {
        const layout = tableLines({
            columns: COLS,
            width: 360,
            breakpoint: 'mobile',
            layouts: [],
        })
        const row = { id: 'g', kind: 'group' as const, depth: 0, label: 'Assets', cells: {} }
        expect(
            rowLayout({ row, cells: {}, isToggle: true, isOpen: false }, layout, 'account')
        ).toEqual({
            kind: 'blocks',
            root: { kind: 'cell', col: 'account', justify: 'start' },
        })
        const full = { number: 'A1', debit: 5, credit: 0 }
        expect(
            rowLayout({ row, cells: full, isToggle: false, isOpen: false }, layout, 'account')
        ).toBe(layout)
    })
    // E2: an opened row with no No. cell (blank on detail rows) keeps the phone lines,
    // so No. is an empty slot under its heading and the amounts stay in their tracks.
    it('keeps the wrapped lines of a detail row whose No. is blank', () => {
        const layout = tableLines({
            columns: COLS,
            width: 360,
            breakpoint: 'mobile',
            layouts: [],
        })
        const row = { id: 'd:1', kind: 'detail' as const, depth: 1, label: 'Sale', cells: {} }
        const cells = { debit: 250, credit: null }
        expect(rowLayout({ row, cells, isToggle: false, isOpen: false }, layout, 'account')).toBe(
            layout
        )
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

// F10: one line when it fits (narrowed if need be), else the nearest narrower
// arrangement, else the automatic lines (R13). WIDE narrows to 864 at best.
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

describe('F10: one line that fits, then the narrower arrangements', () => {
    const lines = (
        width: number,
        breakpoint: 'desktop' | 'tablet' | 'mobile',
        layouts = [] as DisplayLayout[]
    ) => tableLines({ columns: WIDE, width, breakpoint, layouts })

    it('draws a desktop line that fits with its natural widths', () => {
        expect(lines(1300, 'desktop')).toEqual({
            kind: 'one-line',
            widths: fitOneLine(WIDE, 1300, 'desktop'),
        })
        expect(lines(1300, 'desktop')).toMatchObject({
            widths: { customer: 220, note: 128, amount: 128 },
        })
    })

    it('narrows the label, then the text columns, before it wraps', () => {
        expect(lines(1100, 'desktop')).toEqual({
            kind: 'one-line',
            widths: {
                customer: 80,
                status: 127,
                ref: 127,
                team: 127,
                note: 127,
                other: 127,
                amount: 128,
                cost: 128,
                balance: 128,
            },
        })
        expect(lines(864, 'desktop')).toMatchObject({
            kind: 'one-line',
            widths: { note: 80, amount: 128 },
        })
    })

    // Q1 (recommended): an author's arrangement for this width replaces the line.
    it('uses the authored desktop arrangement where there is one, even when the line fits', () => {
        expect(lines(1300, 'desktop', [DESKTOP_LAYOUT, TABLET_LAYOUT])).toEqual(DESKTOP_LINES)
    })

    // The brief's "desktop that wraps to an authored desktop layout": at 800 the
    // narrowed line (864) does not fit, and the desktop arrangement wins over the
    // tablet one. This holds under either answer to Q1.
    it('wraps a desktop too narrow for the line to its own authored arrangement', () => {
        expect(lines(800, 'desktop', [DESKTOP_LAYOUT, TABLET_LAYOUT])).toEqual(DESKTOP_LINES)
    })

    it('wraps a desktop too narrow for the narrowed line to the tablet arrangement, then the phone one, then automatically', () => {
        expect(lines(800, 'desktop', [MOBILE_LAYOUT, TABLET_LAYOUT])).toEqual(TABLET_LINES)
        expect(lines(800, 'desktop', [MOBILE_LAYOUT])).toEqual(MOBILE_LINES)
        const auto = lines(800, 'desktop')
        expect(auto.kind).toBe('blocks')
        expect(auto).toEqual(lines(800, 'tablet'))
        expect(auto).not.toEqual(TABLET_LINES)
    })

    // The label floor is for the label's text, past the deepest row's indent: a
    // grouped phone report that a flat one would narrow onto one line wraps, with
    // its label across the first line.
    it('wraps a grouped phone report with its label on a line of its own', () => {
        const flat = tableLines({ columns: COLS, width: 390, breakpoint: 'mobile', layouts: [] })
        expect(flat).toMatchObject({ kind: 'one-line', widths: { account: 80 } })
        const grouped = tableLines({
            columns: COLS,
            width: 390,
            breakpoint: 'mobile',
            layouts: [],
            levels: { groups: 1, nests: 0 },
        })
        expect(grouped).toEqual(
            tableLines({ columns: COLS, width: 360, breakpoint: 'mobile', layouts: [] })
        )
        const cell = (col: string, justify: 'start' | 'end') => ({
            block: { kind: 'cell', col, justify },
            tracks: 1,
        })
        expect(grouped).toEqual({
            kind: 'blocks',
            root: {
                kind: 'stack',
                parts: [
                    { kind: 'cell', col: 'account', justify: 'start' },
                    {
                        kind: 'side',
                        of: 3,
                        parts: [
                            cell('number', 'start'),
                            cell('debit', 'end'),
                            cell('credit', 'end'),
                        ],
                    },
                ],
            },
        })
    })

    it('wraps a tablet in the same order, never to a wider arrangement', () => {
        expect(lines(700, 'tablet', [DESKTOP_LAYOUT, MOBILE_LAYOUT])).toEqual(MOBILE_LINES)
        expect(lines(700, 'tablet', [DESKTOP_LAYOUT])).toEqual(lines(700, 'mobile'))
        expect(lines(1000, 'tablet', [DESKTOP_LAYOUT])).toMatchObject({ kind: 'one-line' })
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
        breakpoint: 'mobile',
        layouts: [PHONE],
    })
    const item = (kind: ResultRow['kind'], cells: Record<string, CellValue>) => ({
        row: { id: `${kind}:1`, kind, depth: 1, label: 'Before the period', cells },
        cells,
        isToggle: false,
        isOpen: false,
    })
    const gap = { block: { kind: 'gap', lines: 1 } as const, tracks: 1 }

    it('draws an opening row as its label line, then its figures line', () => {
        expect(rowLayout(item('opening', { balance: 1200 }), layout, 'customer')).toEqual({
            kind: 'blocks',
            root: {
                kind: 'stack',
                parts: [label, { kind: 'side', of: 3, parts: [gap, gap, money('balance')] }],
            },
        })
    })

    it('draws a subtotal without its blank text lines, and keeps a blank figure’s track', () => {
        const cells = { amount: 900, cost: 300, balance: null }
        expect(rowLayout(item('subtotal', cells), layout, 'customer')).toEqual({
            kind: 'blocks',
            root: {
                kind: 'stack',
                parts: [
                    label,
                    { kind: 'side', of: 3, parts: [money('amount'), money('cost'), gap] },
                ],
            },
        })
    })

    it('keeps every line of a detail row, and of any row with a value in every column', () => {
        expect(rowLayout(item('detail', { balance: 1200 }), layout, 'customer')).toBe(layout)
        const full = { status: 'open', ref: 'A1', team: 't1', amount: 1, cost: 2, balance: 3 }
        expect(rowLayout(item('total', full), layout, 'customer')).toBe(layout)
    })
})

// F10 over a pivot: the last group level holds the pivot's cells, so its groups
// never open. They draw no caret and no row comes under them, so the label floor
// is the deepest label that the report can really draw.
describe('the label floor of a pivot report', () => {
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
        expect(labelFloor({ groups: 1, nests: 0, pivot: true })).toBe(80)
        expect(labelFloor({ groups: 1, nests: 0 })).toBe(98)
        // Two levels: the outer group's caret (18) passes the pivot rows' indent (16).
        expect(labelFloor({ groups: 2, nests: 0, pivot: true })).toBe(98)
        expect(labelFloor({ groups: 2, nests: 0 })).toBe(114)
        expect(labelFloor({ groups: 3, nests: 0, pivot: true })).toBe(114)
        expect(labelFloor({ groups: 0, nests: 0 })).toBe(80)
    })

    it('fits a one-level pivot on a phone line where a grouped report wraps', () => {
        const at = (levels: { groups: number; nests: number; pivot?: boolean }) =>
            tableLines({ columns: COLS, width: 390, breakpoint: 'mobile', layouts: [], levels })
        expect(at({ groups: 1, nests: 0, pivot: true })).toMatchObject({
            kind: 'one-line',
            widths: { account: 80 },
        })
        expect(at({ groups: 1, nests: 0 })).toMatchObject({ kind: 'blocks' })
    })
})

// F10: a group row's label is not held to the label column's width when the
// cells after it are empty, as its blocks lines draw only its label.
describe('the label span of a one-line row', () => {
    const layout = tableLines({
        columns: COLS,
        width: 1280,
        breakpoint: 'desktop',
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

    it('spans no column of a detail row, or of any other row kind', () => {
        expect(oneLineLabelSpan(item('detail', {}), others)).toBe(0)
        expect(oneLineLabelSpan(item('subtotal', { credit: 1 }), others)).toBe(0)
        expect(oneLineLabelSpan(item('opening', { credit: 1 }), others)).toBe(0)
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
        expect(oneLineLabelWidth({ kind: 'blocks', root: label }, 'account', [])).toBeUndefined()
    })
})
