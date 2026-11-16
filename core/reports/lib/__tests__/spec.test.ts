import { describe, expect, it } from 'vitest'
import {
    brokenProblems,
    CURRENT_SPEC_VERSION,
    canCopyReport,
    displayFitOf,
    headerLinesOf,
    paramDefSchema,
    parseBroken,
    parseFitLength,
    parseSpec,
    reportSpecSchema,
} from '../spec'
import type { RunResult } from '../types'

const FEEDINGS = {
    version: 4,
    source: 'zoo.feedings',
    params: [{ id: 'period', type: 'dateRange', default: { preset: 'thisMonth' } }],
    columns: [
        { id: 'note', path: [{ field: 'note' }] },
        { id: 'species', path: [{ link: 'animal' }, { field: 'species' }], label: 'Species' },
        { id: 'cost', path: [{ field: 'cost' }], aggregate: 'sum' },
        { id: 'half', formula: { op: 'div', args: [{ col: 'cost' }, { value: 2 }], scale: 0 } },
    ],
    filter: {
        op: 'and',
        args: [
            { op: 'between', args: [{ path: [{ field: 'fed' }] }, { param: 'period' }] },
            { op: 'exists', link: [{ link: 'animal' }, { link: 'keepers' }] },
        ],
    },
    sort: [{ col: 'note', dir: 'desc' }],
    groups: [{ id: 'g', col: 'species', subtotals: true }],
}

describe('displayFitOf', () => {
    it('gives the one-line width and the column fits, and nothing else', () => {
        const display = {
            oneLine: { minWidth: '1040px' },
            columns: { note: { minWidth: '12ch' } },
            layouts: [{ width: 'mobile' as const, tracks: 1, cells: [{ col: 'note', line: 1 }] }],
        }
        expect(displayFitOf({ display })).toEqual({
            oneLine: { minWidth: '1040px' },
            columns: { note: { minWidth: '12ch' } },
        })
        expect(displayFitOf({ display: { layouts: display.layouts } })).toEqual({})
        expect(displayFitOf(null)).toEqual({})
    })
})

describe('parseSpec', () => {
    it('reads a stored spec as it is', () => {
        expect(parseSpec(FEEDINGS)).toEqual(FEEDINGS)
    })

    it('reads null parts as absent', () => {
        expect(parseSpec({ ...FEEDINGS, layout: null, pivot: null, filter: null })).not.toBeNull()
    })

    it('reads a pivot and a layout with a computed row', () => {
        const spec = parseSpec({
            ...FEEDINGS,
            pivot: { path: [{ field: 'fed' }], bucket: 'month', cell: 'cost', maxColumns: 12 },
            layout: [
                { section: 'lions', title: 'Lions', key: 'lion' },
                {
                    row: 'net',
                    label: 'Net',
                    formula: { op: 'sub', args: [{ subtotal: 'lions' }, { value: 0 }] },
                },
            ],
        })
        expect(spec?.pivot?.bucket).toBe('month')
        expect(spec?.layout?.[1]?.row).toBe('net')
    })

    it('refuses what is not a spec', () => {
        expect(parseSpec(null)).toBeNull()
        expect(parseSpec('a spec')).toBeNull()
        expect(parseSpec({ version: 4, source: 'zoo.feedings' })).toBeNull()
        expect(parseSpec({ ...FEEDINGS, sql: 'DELETE FROM zoo_feedings' })).toBeNull()
        expect(
            parseSpec({
                ...FEEDINGS,
                columns: [{ id: 'a', path: [{ field: 'x', mode: 'sideways' }] }],
            })
        ).toBeNull()
    })

    it('reads version 4 only', () => {
        expect(parseSpec({ ...FEEDINGS, version: 4 })).not.toBeNull()
        for (const version of [1, 2, 3, 5]) expect(parseSpec({ ...FEEDINGS, version })).toBeNull()
        expect(CURRENT_SPEC_VERSION).toBe(4)
    })

    // E1: optionsFrom.groupOrder (spec.go GroupOrder) is by label, declared, or a path.
    it('reads a group order of each kind and refuses any other', () => {
        const withOrder = (groupOrder: unknown) => ({
            ...FEEDINGS,
            version: 4,
            params: [
                {
                    id: 'animals',
                    type: 'choice',
                    optionsFrom: {
                        source: 'zoo.animals',
                        value: [{ field: 'id' }],
                        label: [{ field: 'name' }],
                        group: [{ field: 'species' }],
                        groupOrder,
                    },
                },
            ],
        })
        for (const order of [
            { by: 'label' },
            { by: 'declared' },
            { by: 'path', path: [{ field: 'weight' }] },
        ]) {
            expect(parseSpec(withOrder(order))?.params?.[0]?.optionsFrom?.groupOrder).toEqual(order)
        }
        for (const bad of [{ by: 'sideways' }, { by: 'path' }, {}, 'declared']) {
            expect(parseSpec(withOrder(bad))).toBeNull()
        }
    })

    // E2: columns[].detail "blank" (spec.go Column.Detail) leaves the column empty on
    // detail rows; "blank" is its one value.
    it('reads a column blank on detail rows and refuses any other detail', () => {
        const withDetail = (detail: unknown) => ({
            ...FEEDINGS,
            version: 4,
            columns: [
                { id: 'note', path: [{ field: 'note' }] },
                { id: 'species', path: [{ link: 'animal' }, { field: 'species' }], detail },
            ],
        })
        expect(parseSpec(withDetail('blank'))?.columns[1]).toEqual({
            id: 'species',
            path: [{ link: 'animal' }, { field: 'species' }],
            detail: 'blank',
        })
        for (const bad of ['hide', 'show', true, { label: 'x' }]) {
            expect(parseSpec(withDetail(bad))).toBeNull()
        }
    })

    // R1 and R2: groups[].inline and groups[].muted are conditions on the group's
    // level, at version 4.
    it('reads a group with inline and muted and keeps both', () => {
        const inline = {
            op: 'eq',
            args: [{ op: 'count', args: [{ path: [{ field: 'id' }] }] }, { value: 1 }],
        }
        const muted = { op: 'not', args: [{ record: 'a', path: [{ field: 'active' }] }] }
        const spec = {
            ...FEEDINGS,
            columns: [...FEEDINGS.columns, { id: 'animal', path: [{ link: 'animal' }] }],
            groups: [
                { id: 'g', col: 'species', subtotals: true, open: true, inline },
                { id: 'a', col: 'animal', muted },
            ],
        }
        const parsed = parseSpec(spec)
        expect(parsed).toEqual(spec)
        expect(parsed?.groups?.[0]?.inline).toEqual(inline)
        expect(parsed?.groups?.[1]?.muted).toEqual(muted)
        expect(parseSpec(FEEDINGS)?.groups?.[0]).toEqual({
            id: 'g',
            col: 'species',
            subtotals: true,
        })
    })

    it('refuses a spec from a newer version of the app as unreadable', () => {
        expect(parseSpec({ ...FEEDINGS, version: CURRENT_SPEC_VERSION })).not.toBeNull()
        expect(parseSpec({ ...FEEDINGS, version: CURRENT_SPEC_VERSION + 1 })).toBeNull()
    })

    it('reads every v2 part', () => {
        const spec = {
            version: 4,
            source: 'zoo.feedings',
            params: [
                {
                    id: 'animals',
                    type: 'choice',
                    multiple: true,
                    optionsFrom: {
                        source: 'zoo.animals',
                        value: [{ field: 'id' }],
                        label: [{ field: 'name' }],
                        group: [{ field: 'species' }],
                        where: {
                            op: 'eq',
                            args: [{ path: [{ field: 'active' }] }, { value: true }],
                        },
                    },
                },
                { id: 'budget', type: 'amount', default: { amount: 150000, currency: 'USD' } },
            ],
            columns: [
                { id: 'animal', path: [{ link: 'animal' }], labelField: 'name' },
                { id: 'weight', formula: { record: 'g', path: [{ field: 'weight' }] } },
                { id: 'cost', path: [{ field: 'cost' }], aggregate: 'sum', total: 'rows' },
                {
                    id: 'site',
                    formula: {
                        op: 'lookup',
                        source: 'zoo.sites',
                        value: { path: [{ field: 'name' }] },
                        pick: 'max',
                    },
                },
            ],
            groups: [
                {
                    id: 'g',
                    col: 'animal',
                    label: { record: 'g', path: [{ field: 'name' }] },
                    having: { op: 'gt', args: [{ col: 'cost' }, { value: 0 }] },
                    fill: {
                        where: {
                            op: 'eq',
                            args: [{ path: [{ field: 'active' }] }, { value: true }],
                        },
                    },
                    open: true,
                },
            ],
            sort: [
                { formula: { record: 'g', path: [{ field: 'weight' }] }, dir: 'desc' },
                { col: 'cost' },
            ],
            detail: { label: { path: [{ field: 'note' }] } },
            display: {
                layouts: [
                    {
                        width: 'mobile',
                        tracks: 3,
                        cells: [
                            { col: 'animal', line: 1, span: 3 },
                            { col: 'cost', line: 2, justify: 'end' },
                        ],
                    },
                ],
            },
        }
        expect(parseSpec(spec)).toEqual(spec)
        expect(parseSpec({ ...spec, columns: [{ id: 'x', path: [], total: 'some' }] })).toBeNull()
        expect(
            parseSpec({
                ...spec,
                display: { layouts: [{ width: 'watch', tracks: 1, cells: [] }] },
            })
        ).toBeNull()
        expect(
            parseSpec({ ...spec, columns: [{ id: 'x', formula: { op: 'lookup', pick: 'avg' } }] })
        ).toBeNull()
    })

    it('reads a pivot over a link by its label field', () => {
        const pivot = { path: [{ link: 'animal' }], cell: 'cost', labelField: 'name' }
        expect(parseSpec({ ...FEEDINGS, pivot })?.pivot).toEqual(pivot)
    })

    it('reads a pivot over a range with its total after the pivot columns', () => {
        const spec = {
            ...FEEDINGS,
            pivot: {
                path: [{ field: 'fed' }],
                bucket: 'month',
                cell: 'cost',
                range: 'period',
                order: 'spec',
            },
        }
        expect(parseSpec(spec)).toEqual(spec)
        expect(parseSpec({ ...spec, pivot: { ...spec.pivot, order: 'after' } })).toBeNull()
    })

    it('reads a percent of a section with the section it names', () => {
        const share = {
            id: 'share',
            formula: {
                op: 'percentOfSection',
                args: [{ col: 'cost' }],
                subtotal: 'income',
                scale: 1,
            },
        }
        const spec = { ...FEEDINGS, columns: [...FEEDINGS.columns, share] }
        expect(parseSpec(spec)).toEqual(spec)
    })
})

describe('canCopyReport', () => {
    const mark = { status: 422, problems: { source: { code: 'unknown_source', message: 'x' } } }

    it('offers a copy of a readable report with no broken mark', () => {
        expect(canCopyReport({ spec: FEEDINGS, broken: null })).toBe(true)
    })

    it('does not offer a copy of a broken or unreadable report', () => {
        expect(canCopyReport({ spec: FEEDINGS, broken: mark })).toBe(false)
        expect(canCopyReport({ spec: { ...FEEDINGS, version: 99 }, broken: null })).toBe(false)
        expect(canCopyReport({ spec: null, broken: null })).toBe(false)
    })
})

describe('parseBroken', () => {
    it('lists each problem by spec path, in path order', () => {
        const mark = parseBroken({
            status: 422,
            problems: {
                source: { code: 'unknown_source', message: 'the source "zoo.gone" does not exist' },
                'columns.0.path.0.field': { code: 'unknown_field', message: 'no field "x"' },
            },
            more: 2,
        })
        expect(mark?.status).toBe(422)
        expect(mark?.more).toBe(2)
        expect(mark && brokenProblems(mark)).toEqual([
            { path: 'columns.0.path.0.field', code: 'unknown_field', message: 'no field "x"' },
            {
                path: 'source',
                code: 'unknown_source',
                message: 'the source "zoo.gone" does not exist',
            },
        ])
    })

    it('reads no mark as null', () => {
        expect(parseBroken(null)).toBeNull()
        expect(parseBroken(undefined)).toBeNull()
        expect(parseBroken({ status: 'bad' })).toBeNull()
    })
})

// F1/F3 inside a lookup: the animal's cost before the period.
const BEFORE_PERIOD = {
    op: 'coalesce',
    args: [
        {
            op: 'lookup',
            source: 'zoo.feedings',
            where: {
                op: 'and',
                args: [
                    {
                        op: 'eq',
                        args: [
                            { path: [{ link: 'animal' }] },
                            { record: 'byAnimal', path: [{ field: 'id' }] },
                        ],
                    },
                    {
                        op: 'lt',
                        args: [{ path: [{ field: 'fed' }] }, { param: 'period', bound: 'from' }],
                    },
                ],
            },
            value: { path: [{ field: 'cost' }] },
            pick: 'sum',
        },
        { value: 0 },
    ],
}

// The head's v3 example: every new part once.
const V3 = {
    version: 4,
    source: 'zoo.feedings',
    params: [{ id: 'period', type: 'dateRange', label: 'Period', default: { preset: 'allDates' } }],
    columns: [
        { id: 'animal', label: 'Animal', path: [{ link: 'animal' }] },
        { id: 'fed', label: 'Fed', path: [{ field: 'fed' }] },
        { id: 'cost', label: 'Cost', path: [{ field: 'cost' }], aggregate: 'sum' },
        {
            id: 'sameDay',
            label: 'Same day',
            formula: {
                op: 'lookup',
                source: 'zoo.feedings',
                where: {
                    op: 'and',
                    args: [
                        {
                            op: 'eq',
                            args: [{ path: [{ link: 'animal' }] }, { outer: [{ link: 'animal' }] }],
                        },
                        {
                            op: 'eq',
                            args: [{ path: [{ field: 'fed' }] }, { outer: [{ field: 'fed' }] }],
                        },
                    ],
                },
                value: { path: [{ field: 'id' }] },
                pick: 'count',
            },
        },
        {
            id: 'balance',
            label: 'Balance',
            formula: {
                op: 'add',
                args: [BEFORE_PERIOD, { op: 'runningSum', args: [{ path: [{ field: 'cost' }] }] }],
            },
            rollup: {
                op: 'add',
                args: [{ op: 'sum', args: [{ path: [{ field: 'cost' }] }] }, { value: 0 }],
            },
        },
    ],
    filter: { op: 'between', args: [{ path: [{ field: 'fed' }] }, { param: 'period' }] },
    groups: [
        {
            id: 'byAnimal',
            col: 'animal',
            open: true,
            subtotals: true,
            opening: { label: 'Before the period', cells: { balance: BEFORE_PERIOD } },
        },
    ],
    sort: [{ col: 'fed' }],
}

describe('the parts of version 3', () => {
    it('reads every v3 part as it is', () => {
        expect(parseSpec(V3)).toEqual(V3)
    })

    it('reads a v3 spec with none of the new parts', () => {
        expect(parseSpec({ ...FEEDINGS, version: 4 })).toEqual({ ...FEEDINGS, version: 4 })
    })

    it('reads an opening row with no cells', () => {
        const groups = [{ id: 'g', col: 'species', opening: { label: 'Before' } }]
        expect(parseSpec({ ...FEEDINGS, version: 4, groups })?.groups?.[0]?.opening).toEqual({
            label: 'Before',
        })
    })

    it('refuses a pick, a bound or an opening label the engine refuses', () => {
        const lookup = (pick: string) => ({
            ...FEEDINGS,
            version: 4,
            columns: [
                {
                    id: 'x',
                    formula: {
                        op: 'lookup',
                        source: 'zoo.animals',
                        value: { path: [{ field: 'weight' }] },
                        pick,
                    },
                },
            ],
        })
        for (const pick of ['min', 'max', 'sum', 'count'])
            expect(parseSpec(lookup(pick))).not.toBeNull()
        expect(parseSpec(lookup('avg'))).toBeNull()

        const bound = (b: string) => ({
            ...FEEDINGS,
            version: 4,
            filter: {
                op: 'lt',
                args: [{ path: [{ field: 'fed' }] }, { param: 'period', bound: b }],
            },
        })
        expect(parseSpec(bound('to'))).not.toBeNull()
        expect(parseSpec(bound('mid'))).toBeNull()

        const opening = (label: string) => ({
            ...FEEDINGS,
            version: 4,
            groups: [{ id: 'g', col: 'species', opening: { label } }],
        })
        expect(parseSpec(opening(''))).toBeNull()
        expect(parseSpec(opening('x'.repeat(201)))).toBeNull()
        expect(parseSpec(opening('x'.repeat(200)))).not.toBeNull()
    })

    it('counts an opening label in characters, as the engine does, not in UTF-16 units', () => {
        const opening = (label: string) => ({
            ...FEEDINGS,
            version: 4,
            groups: [{ id: 'g', col: 'species', opening: { label } }],
        })
        // A CJK Extension B character is one character and two UTF-16 units.
        const cjk = (n: number) => '𠀋'.repeat(n)
        expect(parseSpec(opening(cjk(200)))).not.toBeNull()
        expect(parseSpec(opening(cjk(201)))).toBeNull()
    })

    it('refuses an outer leaf that is not a path of hops, and an opening part it does not know', () => {
        const outer = (value: unknown) => ({
            ...FEEDINGS,
            version: 4,
            columns: [{ id: 'x', formula: { outer: value } }],
        })
        expect(parseSpec(outer([{ field: 'fed' }]))).not.toBeNull()
        expect(parseSpec(outer('fed'))).toBeNull()
        const groups = [{ id: 'g', col: 'species', opening: { label: 'Before', rows: 2 } }]
        expect(parseSpec({ ...FEEDINGS, version: 4, groups })).toBeNull()
    })
})

describe('column fit (spec version 4)', () => {
    const FIT = {
        ...FEEDINGS,
        display: {
            oneLine: { minWidth: '1040px' },
            columns: {
                cost: { flexGrow: 0 },
                note: {
                    minWidth: '7.5ch',
                    maxWidth: '40%',
                    whiteSpace: 'normal',
                    lineClamp: 2,
                    textOverflow: 'ellipsis',
                },
            },
            layouts: [
                {
                    width: 'tablet',
                    minWidth: '600px',
                    tracks: 2,
                    cells: [{ col: 'note', line: 1 }],
                },
            ],
        },
    }

    const WRAP = { whiteSpace: 'normal', lineClamp: 2 }
    const ANYWHERE = { overflowWrap: 'anywhere' }

    it('reads every fit setting as it is', () => {
        expect(parseSpec(FIT)).toEqual(FIT)
    })

    it('refuses a length with no unit, a width gate not in px, and a value Go never stores', () => {
        const withDisplay = (display: unknown) => parseSpec({ ...FEEDINGS, display })
        expect(withDisplay({ columns: { note: { minWidth: '12' } } })).toBeNull()
        expect(withDisplay({ oneLine: { minWidth: '60ch' } })).toBeNull()
        expect(withDisplay({ oneLine: {} })).toBeNull()
        expect(
            withDisplay({ layouts: [{ width: 'tablet', minWidth: '50%', tracks: 1, cells: [] }] })
        ).toBeNull()
        expect(withDisplay({ columns: { note: { textOverflow: 'clip' } } })).toBeNull()
        expect(withDisplay({ columns: { note: { lineClamp: 5 } } })).toBeNull()
        expect(withDisplay({ columns: { note: { colour: 'red' } } })).toBeNull()
    })

    it('takes overflowWrap anywhere only on a column that wraps', () => {
        const withNote = (note: unknown) =>
            parseSpec({ ...FEEDINGS, display: { columns: { note } } })
        const breaks = { ...FEEDINGS, display: { columns: { note: { ...WRAP, ...ANYWHERE } } } }
        expect(parseSpec(breaks)).toEqual(breaks)
        expect(withNote({ ...WRAP, overflowWrap: 'break-word' })).toBeNull()
        expect(withNote({ ...WRAP, overflowWrap: 'normal' })).toBeNull()
        expect(withNote(ANYWHERE)).toBeNull()
        expect(withNote({ whiteSpace: 'nowrap', ...ANYWHERE })).toBeNull()
    })

    it('refuses overflowWrap on a column of one line, which never breaks a word', () => {
        const withNote = (note: unknown) =>
            parseSpec({ ...FEEDINGS, display: { columns: { note } } })
        expect(withNote({ whiteSpace: 'normal', lineClamp: 1, ...ANYWHERE })).toBeNull()
        expect(withNote({ whiteSpace: 'normal', lineClamp: 1 })).not.toBeNull()
        expect(withNote({ whiteSpace: 'normal', lineClamp: 3, ...ANYWHERE })).not.toBeNull()
    })

    it('files an overflowWrap refusal on the fit itself, where the builder reads it', () => {
        const pathsOf = (note: unknown) => {
            const parsed = reportSpecSchema.safeParse({
                ...FEEDINGS,
                display: { columns: { note } },
            })
            return parsed.success ? [] : parsed.error.issues.map(issue => issue.path.join('.'))
        }
        expect(pathsOf(ANYWHERE)).toEqual(['display.columns.note'])
        expect(pathsOf({ whiteSpace: 'nowrap', ...ANYWHERE })).toEqual(['display.columns.note'])
        expect(pathsOf({ whiteSpace: 'normal', lineClamp: 1, ...ANYWHERE })).toEqual([
            'display.columns.note',
        ])
    })

    it('parses a length into its number and unit', () => {
        expect(parseFitLength('7.5ch')).toEqual({ value: 7.5, unit: 'ch' })
        expect(parseFitLength('600px')).toEqual({ value: 600, unit: 'px' })
        expect(parseFitLength('40%')).toEqual({ value: 40, unit: '%' })
        expect(parseFitLength('max-content')).toBeNull()
        expect(parseFitLength('12')).toBeNull()
    })
})

describe('statement fields (G7)', () => {
    const base = {
        version: 4,
        source: 'zoo.feedings',
        columns: [
            { id: 'animal', path: [{ link: 'animal' }] },
            { id: 'sp', path: [{ link: 'animal' }, { field: 'species' }], hidden: true },
        ],
    }

    it('parses a hidden column', () => {
        expect(parseSpec(base)?.columns[1]?.hidden).toBe(true)
    })

    it('keeps a spec without hidden valid', () => {
        const { hidden: _hidden, ...plain } = base.columns[1]
        expect(parseSpec({ ...base, columns: [base.columns[0], plain] })).not.toBeNull()
    })
})

describe('statement fields (G1)', () => {
    it('parses blank: skip on a group and refuses any other value', () => {
        const spec = {
            version: 4,
            source: 'zoo.feedings',
            columns: [{ id: 'a', path: [{ field: 'name' }] }],
            groups: [
                { id: 's', col: 'a' },
                { id: 'r', col: 'a', blank: 'skip' },
                { id: 'x', col: 'a' },
            ],
        }
        expect(parseSpec(spec)?.groups?.[1]?.blank).toBe('skip')
        expect(parseSpec({ ...spec, groups: [{ id: 'r', col: 'a', blank: 'keep' }] })).toBeNull()
    })
})

describe('statement fields (G5)', () => {
    it('parses hideEmpty on a group', () => {
        const spec = {
            version: 4,
            source: 'zoo.feedings',
            columns: [{ id: 'a', path: [{ field: 'name' }] }],
            groups: [
                { id: 's', col: 'a', hideEmpty: true },
                { id: 'x', col: 'a' },
            ],
        }
        expect(parseSpec(spec)?.groups?.[0]?.hideEmpty).toBe(true)
    })
})

describe('statement fields (G3)', () => {
    it('parses a heading formula on a column', () => {
        const spec = {
            version: 4,
            source: 'zoo.feedings',
            params: [{ id: 'asOf', type: 'asOf' }],
            columns: [
                { id: 'a', path: [{ field: 'name' }] },
                { id: 'c', path: [{ field: 'cost' }], heading: { param: 'asOf' } },
            ],
        }
        expect(parseSpec(spec)?.columns[1]?.heading).toEqual({ param: 'asOf' })
    })
})

describe('statement fields (G4, G8)', () => {
    it('parses within and cells on a computed row', () => {
        const spec = {
            version: 4,
            source: 'zoo.feedings',
            columns: [{ id: 'a', path: [{ field: 'name' }] }],
            groups: [{ id: 'g', col: 'a' }],
            layout: [
                { section: 'eq', title: 'Equity', key: 'equity' },
                { row: 're', label: 'Retained', within: 'eq', cells: { cost: { value: 1 } } },
            ],
        }
        const row = parseSpec(spec)?.layout?.[1]
        expect(row?.within).toBe('eq')
        expect(row?.cells).toEqual({ cost: { value: 1 } })
    })
})

describe('drills and opened rows (D1, D2)', () => {
    const drill = {
        report: 'zoo.drill_target',
        levels: {
            g: {
                species: { groupKey: 'g' },
                period: { param: 'period' },
                animals: { group: { groupKey: 'g' } },
                fedOn: { op: 'max', args: [{ path: [{ field: 'fed' }] }] },
            },
        },
        pivotCells: {
            period: { range: { from: { pivotBound: 'from' }, to: { pivotBound: 'to' } } },
        },
    }
    const withDrill = (value: unknown) => ({
        ...FEEDINGS,
        columns: FEEDINGS.columns.map(c => (c.id === 'cost' ? { ...c, drill: value } : c)),
    })

    it('parses a column drill in every value form', () => {
        expect(parseSpec(withDrill(drill))).toEqual(withDrill(drill))
    })

    it('refuses a drill value with an unknown key', () => {
        const bad = {
            ...drill,
            pivotCells: { period: { range: { from: { value: 1 }, to: { value: 2 }, x: 1 } } },
        }
        expect(parseSpec(withDrill(bad))).toBeNull()
    })

    it('refuses a pivotBound that is not from or to', () => {
        const bad = {
            report: 'zoo.drill_target',
            levels: { g: { fedOn: { pivotBound: 'middle' } } },
        }
        expect(parseSpec(withDrill(bad))).toBeNull()
    })

    it('parses an empty detail, which only the engine refuses at bind', () => {
        expect(parseSpec({ ...FEEDINGS, detail: {} })?.detail).toEqual({})
    })

    it('parses detail.open with or without a label', () => {
        const open = {
            op: 'concat',
            args: [{ value: 'zoo/feedings/' }, { path: [{ field: 'note' }] }],
        }
        expect(parseSpec({ ...FEEDINGS, detail: { open } })?.detail).toEqual({ open })
        expect(
            parseSpec({ ...FEEDINGS, detail: { label: { col: 'note' }, open } })?.detail
        ).toEqual({
            label: { col: 'note' },
            open,
        })
    })

    it('keeps parsing a spec without them', () => {
        expect(parseSpec(FEEDINGS)).toEqual(FEEDINGS)
    })
})

// N1: a choice param's header line names the chosen option under the period.
describe('a header line on a choice param (N1)', () => {
    const BASIS = {
        id: 'basis',
        type: 'choice',
        default: 'accrual',
        header: '{label} basis',
        options: [
            { value: 'accrual', label: 'Accrual' },
            { value: 'cash', label: 'Cash' },
        ],
    }
    const PLAIN = { id: 'plain', type: 'choice', options: [{ value: 'hay', label: 'Hay' }] }
    const KIND = {
        id: 'kind',
        type: 'choice',
        header: 'Feed: {label}',
        options: [
            { value: 'hay', label: 'Hay' },
            { value: 'fish', label: 'Fish' },
        ],
    }
    const result = (headerLines: string[]): RunResult => ({
        columns: [],
        rows: [],
        notices: [],
        topics: [],
        page: null,
        params: [],
        headerLines,
    })

    it('accepts a header on a choice param and keeps it through parse', () => {
        expect(paramDefSchema.parse(BASIS)).toEqual(BASIS)
        const withBasis = { ...FEEDINGS, params: [...FEEDINGS.params, BASIS] }
        expect(parseSpec(withBasis)).toEqual(withBasis)
        expect(parseSpec(withBasis)?.params?.[1]?.header).toBe('{label} basis')
    })

    it('pairs each line with the choice param that has a header, in param order', () => {
        const spec = parseSpec({ ...FEEDINGS, params: [...FEEDINGS.params, BASIS, PLAIN, KIND] })
        expect(spec).not.toBeNull()
        if (!spec) return
        expect(headerLinesOf(spec, result(['Cash basis', 'Feed: Fish']))).toEqual([
            { id: 'basis', text: 'Cash basis' },
            { id: 'kind', text: 'Feed: Fish' },
        ])
    })

    it('gives no lines for a spec with no header', () => {
        const spec = parseSpec(FEEDINGS)
        expect(spec).not.toBeNull()
        if (!spec) return
        expect(headerLinesOf(spec, result([]))).toEqual([])
    })
})
