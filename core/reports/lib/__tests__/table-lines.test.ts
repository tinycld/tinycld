import { describe, expect, it } from 'vitest'
import { z } from 'zod'
import table from '../fixtures/display-cells.json'
import { type DisplayCell, displayCellSchema } from '../spec'
import {
    PINWHEEL_MESSAGE,
    PIVOT_NOTICE,
    placeCells,
    rowLayout,
    startingLayout,
    tableLines,
} from '../table-lines'
import type { ResultColumn } from '../types'

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
    it('keeps one line on desktop and wherever the columns fit', () => {
        expect(
            tableLines({
                columns: COLS,
                width: 1280,
                breakpoint: 'desktop',
                layouts: [],
                oneLineFits: false,
            })
        ).toEqual({ kind: 'one-line' })
        expect(
            tableLines({
                columns: COLS,
                width: 900,
                breakpoint: 'tablet',
                layouts: [],
                oneLineFits: true,
            })
        ).toEqual({ kind: 'one-line' })
    })
    it('wraps a phone row: the label across line 1, the amounts in aligned tracks below', () => {
        expect(
            tableLines({
                columns: COLS,
                width: 390,
                breakpoint: 'mobile',
                layouts: [],
                oneLineFits: false,
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
                oneLineFits: false,
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
            oneLineFits: false,
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
            oneLineFits: false,
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
                oneLineFits: false,
                layouts: [mobile],
            })
        ).toEqual({ kind: 'one-line' })
        const auto = tableLines({
            columns: COLS,
            width: 390,
            breakpoint: 'mobile',
            layouts: [],
            oneLineFits: false,
        })
        const refused = { ...mobile, cells: [{ col: 'debit', line: 9 }] }
        expect(
            tableLines({
                columns: COLS,
                width: 390,
                breakpoint: 'mobile',
                oneLineFits: false,
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
            oneLineFits: false,
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
            oneLineFits: false,
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
            oneLineFits: false,
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
            oneLineFits: false,
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
            oneLineFits: false,
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
            oneLineFits: false,
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
            width: 390,
            breakpoint: 'mobile',
            layouts: [],
            oneLineFits: false,
        })
        const row = { id: 'g', kind: 'group' as const, depth: 0, label: 'Assets', cells: {} }
        expect(
            rowLayout({ row, cells: {}, isToggle: true, isOpen: false }, layout, 'account')
        ).toEqual({
            kind: 'blocks',
            root: { kind: 'cell', col: 'account', justify: 'start' },
        })
        expect(
            rowLayout(
                { row, cells: { debit: 5 }, isToggle: false, isOpen: false },
                layout,
                'account'
            )
        ).toBe(layout)
    })
    // E2: an opened row with no No. cell (blank on detail rows) keeps the phone lines,
    // so No. is an empty slot under its heading and the amounts stay in their tracks.
    it('keeps the wrapped lines of a detail row whose No. is blank', () => {
        const layout = tableLines({
            columns: COLS,
            width: 390,
            breakpoint: 'mobile',
            layouts: [],
            oneLineFits: false,
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
