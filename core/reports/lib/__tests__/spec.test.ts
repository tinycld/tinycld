import { describe, expect, it } from 'vitest'
import {
    brokenProblems,
    CURRENT_SPEC_VERSION,
    canCopyReport,
    parseBroken,
    parseSpec,
} from '../spec'

const FEEDINGS = {
    version: 1,
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
        expect(parseSpec({ version: 1, source: 'zoo.feedings' })).toBeNull()
        expect(parseSpec({ ...FEEDINGS, sql: 'DELETE FROM zoo_feedings' })).toBeNull()
        expect(
            parseSpec({
                ...FEEDINGS,
                columns: [{ id: 'a', path: [{ field: 'x', mode: 'sideways' }] }],
            })
        ).toBeNull()
    })

    it('reads a version 1 to 4 spec and refuses version 5', () => {
        expect(parseSpec({ ...FEEDINGS, version: 1 })).not.toBeNull()
        expect(parseSpec({ ...FEEDINGS, version: 4 })).not.toBeNull()
        expect(parseSpec({ ...FEEDINGS, version: 5 })).toBeNull()
        expect(CURRENT_SPEC_VERSION).toBe(4)
    })

    // E1: optionsFrom.groupOrder (spec.go GroupOrder) is by label, declared, or a path.
    it('reads a group order of each kind and refuses any other', () => {
        const withOrder = (groupOrder: unknown) => ({
            ...FEEDINGS,
            version: 2,
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
            version: 2,
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

    it('refuses a spec from a newer version of the app as unreadable', () => {
        expect(parseSpec({ ...FEEDINGS, version: CURRENT_SPEC_VERSION })).not.toBeNull()
        expect(parseSpec({ ...FEEDINGS, version: CURRENT_SPEC_VERSION + 1 })).toBeNull()
    })

    it('reads every v2 part', () => {
        const spec = {
            version: 2,
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
    version: 3,
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

describe('spec version 3', () => {
    it('reads every v3 part as it is', () => {
        expect(parseSpec(V3)).toEqual(V3)
    })

    it('reads a v3 spec with none of the new parts', () => {
        expect(parseSpec({ ...FEEDINGS, version: 3 })).toEqual({ ...FEEDINGS, version: 3 })
    })

    it('reads an opening row with no cells', () => {
        const groups = [{ id: 'g', col: 'species', opening: { label: 'Before' } }]
        expect(parseSpec({ ...FEEDINGS, version: 3, groups })?.groups?.[0]?.opening).toEqual({
            label: 'Before',
        })
    })

    it('refuses a pick, a bound or an opening label the engine refuses', () => {
        const lookup = (pick: string) => ({
            ...FEEDINGS,
            version: 3,
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
            version: 3,
            filter: {
                op: 'lt',
                args: [{ path: [{ field: 'fed' }] }, { param: 'period', bound: b }],
            },
        })
        expect(parseSpec(bound('to'))).not.toBeNull()
        expect(parseSpec(bound('mid'))).toBeNull()

        const opening = (label: string) => ({
            ...FEEDINGS,
            version: 3,
            groups: [{ id: 'g', col: 'species', opening: { label } }],
        })
        expect(parseSpec(opening(''))).toBeNull()
        expect(parseSpec(opening('x'.repeat(201)))).toBeNull()
        expect(parseSpec(opening('x'.repeat(200)))).not.toBeNull()
    })

    it('counts an opening label in characters, as the engine does, not in UTF-16 units', () => {
        const opening = (label: string) => ({
            ...FEEDINGS,
            version: 3,
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
            version: 3,
            columns: [{ id: 'x', formula: { outer: value } }],
        })
        expect(parseSpec(outer([{ field: 'fed' }]))).not.toBeNull()
        expect(parseSpec(outer('fed'))).toBeNull()
        const groups = [{ id: 'g', col: 'species', opening: { label: 'Before', rows: 2 } }]
        expect(parseSpec({ ...FEEDINGS, version: 3, groups })).toBeNull()
    })
})
