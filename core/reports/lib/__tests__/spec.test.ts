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

    it('reads a version 1 spec and a version 2 spec', () => {
        expect(parseSpec({ ...FEEDINGS, version: 1 })).not.toBeNull()
        expect(parseSpec({ ...FEEDINGS, version: 2 })).not.toBeNull()
        expect(parseSpec({ ...FEEDINGS, version: 3 })).toBeNull()
    })

    it('refuses a spec from a newer version of the app as unreadable', () => {
        expect(parseSpec({ ...FEEDINGS, version: CURRENT_SPEC_VERSION })).not.toBeNull()
        expect(parseSpec({ ...FEEDINGS, version: CURRENT_SPEC_VERSION + 1 })).toBeNull()
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
