import { describe, expect, it } from 'vitest'
import type { ParamDef } from '../../spec'
import { type BuilderDraft, emptyDraft } from '../draft'
import { emptyGroup } from '../filter'
import { columnChoices, formulaContext, recordGroups } from '../formula-context'
import { contextAt } from '../formula-ops'
import { CATALOG, SALES_KEY, TEAMS_KEY } from './catalog-fixture'

const PERIOD: ParamDef = { id: 'period', type: 'dateRange', label: 'Period' }

function draft(): BuilderDraft {
    const sales = CATALOG.get(SALES_KEY)
    if (!sales) throw new Error('fixture')
    return {
        ...emptyDraft(sales),
        params: [PERIOD],
        columns: [
            {
                kind: 'path',
                id: 'team',
                label: 'Team',
                path: [{ link: 'team' }],
                aggregate: '',
                scale: null,
                format: '',
                total: 'recompute',
                explicitTotal: false,
                labelField: '',
            },
            {
                kind: 'path',
                id: 'amount',
                label: 'Amount',
                path: [{ field: 'amount' }],
                aggregate: 'sum',
                scale: null,
                format: '',
                total: 'recompute',
                explicitTotal: false,
                labelField: '',
            },
        ],
        groups: [
            {
                id: 'byTeam',
                col: 'team',
                bucket: '',
                subtotals: false,
                open: false,
                label: null,
                having: emptyGroup(),
                fill: { on: false, where: emptyGroup() },
            },
        ],
        layout: [
            { kind: 'section', id: 'open', title: 'Open', key: 'open' },
            {
                kind: 'row',
                id: 'gap',
                label: 'Gap',
                formula: { kind: 'editable', node: { value: 0 } },
            },
        ],
    }
}

describe('what a formula reads where it is edited', () => {
    it('a group by a link carries its record', () => {
        expect(recordGroups(draft(), CATALOG)).toEqual([
            { group: 'byTeam', label: 'Team', source: TEAMS_KEY },
        ])
    })

    it('a column reads the other columns, paths, records, params, totals and running values', () => {
        const ctx = formulaContext(draft(), CATALOG, { kind: 'column', selfKey: 'amount' })
        expect(ctx.values.map(v => v.key)).toEqual(['team'])
        expect(ctx).toMatchObject({
            leaf: 'col',
            source: SALES_KEY,
            totals: true,
            windows: true,
            lookups: true,
        })
        expect(ctx.records).toHaveLength(1)
        expect(ctx.params).toEqual([{ id: 'period', label: 'Period', type: 'dateRange' }])
    })

    it('a group level and a row label read totals but no running value', () => {
        for (const kind of ['level', 'detail'] as const) {
            const ctx = formulaContext(draft(), CATALOG, { kind })
            expect(ctx.values.map(v => v.key)).toEqual(['team', 'amount'])
            expect(ctx).toMatchObject({ leaf: 'col', totals: true, windows: false, lookups: true })
        }
    })

    // A computed row is worked out once per column (layout.go); one amount column
    // makes its subtotals amounts, so a division of one keeps whole cents.
    it('a layout row reads the items before it, as amounts when a column is one', () => {
        const ctx = formulaContext(draft(), CATALOG, { kind: 'layout', index: 1 })
        expect(ctx).toMatchObject({
            leaf: 'subtotal',
            source: null,
            records: [],
            params: [],
            totals: false,
            windows: false,
        })
        expect(ctx.values).toEqual([{ key: 'open', label: 'Open', type: 'money' }])
        const counts = draft()
        counts.columns = counts.columns.map(c =>
            c.id === 'amount' ? { ...c, aggregate: 'count' as const } : c
        )
        const counted = formulaContext(counts, CATALOG, { kind: 'layout', index: 1 })
        expect(counted.values).toEqual([{ key: 'open', label: 'Open', type: 'number' }])
    })

    // F16 / R21: a computed row may look a value up; the lookup reads params
    // (lookup.go lookupCtx.param) though the row itself reads none (layout.go param).
    it('a layout row looks values up, and its lookups read the params', () => {
        const ctx = formulaContext(draft(), CATALOG, { kind: 'layout', index: 1 })
        expect(ctx.lookups).toBe(true)
        const lookup = {
            op: 'lookup',
            source: TEAMS_KEY,
            where: { value: true },
            value: { path: [{ field: 'name' }] },
        }
        const inside = contextAt(ctx, lookup, [0])
        expect(inside.source).toBe(TEAMS_KEY)
        expect(inside.params.map(p => p.id)).toEqual(['period'])
    })
})

describe('the type of a column', () => {
    // Every section reads columnChoices with the groups and params, so a formula
    // reading a param or a group record has one type everywhere.
    it('types a formula by the param and the group record it reads', () => {
        const base = draft()
        const formula = (id: string, node: object) => ({
            kind: 'formula' as const,
            id,
            label: id,
            aggregate: '' as const,
            scale: null,
            format: '',
            total: 'recompute' as const,
            explicitTotal: false,
            labelField: '',
            formula: { kind: 'editable' as const, node },
        })
        const typed: BuilderDraft = {
            ...base,
            params: [...base.params, { id: 'least', type: 'amount', label: 'Least' }],
            columns: [
                ...base.columns,
                formula('asked', { param: 'least' }),
                formula('team_id', { record: 'byTeam', path: [{ field: 'id' }] }),
            ],
        }
        const choices = columnChoices(typed, CATALOG)
        expect(choices.find(c => c.key === 'asked')).toMatchObject({
            type: 'money',
            isNumeric: true,
        })
        expect(choices.find(c => c.key === 'team_id')?.type).toBe('relation')
    })
})
