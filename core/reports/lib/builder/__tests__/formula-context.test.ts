import { describe, expect, it } from 'vitest'
import type { ParamDef, SpecHop, SpecNode } from '../../spec'
import {
    type BuilderDraft,
    type ColumnDraft,
    emptyDraft,
    type FormulaDraft,
    groupTarget,
    levelScope,
} from '../draft'
import { emptyGroup } from '../filter'
import {
    columnChoices,
    columnTarget,
    formulaContext,
    levelValueColumns,
    linkTarget,
    readsColumn,
    recordGroups,
    rowsTotalColumns,
    sectionPercentColumns,
    sortColumns,
    windowColumns,
} from '../formula-context'
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

// Two levels, by status then by team, over columns with and without a value on a
// group row (levels.go readsRowValue), and two running values.
function levelsDraft(): BuilderDraft {
    const base = draft()
    const common = {
        aggregate: '' as const,
        scale: null,
        format: '',
        total: 'recompute' as const,
        explicitTotal: false,
        labelField: '',
    }
    const column = (
        id: string,
        rest:
            | { kind: 'path'; path: SpecHop[]; aggregate?: 'sum'; total?: 'rows' }
            | { kind: 'formula'; formula: FormulaDraft }
    ): ColumnDraft => ({ ...common, id, label: id, ...rest })
    const formula = (node: SpecNode): FormulaDraft => ({ kind: 'editable', node })
    const level = (id: string, col: string, bucket: '' | 'month' = '') => ({
        id,
        col,
        bucket,
        subtotals: false,
        open: false,
        label: null,
        having: emptyGroup(),
        fill: { on: false, where: emptyGroup() },
    })
    return {
        ...base,
        columns: [
            column('team', { kind: 'path', path: [{ link: 'team' }] }),
            column('status', { kind: 'path', path: [{ field: 'status' }] }),
            column('customer', { kind: 'path', path: [{ field: 'customer' }] }),
            column('day', { kind: 'path', path: [{ field: 'day' }] }),
            column('amount', { kind: 'path', path: [{ field: 'amount' }], aggregate: 'sum' }),
            column('qty', { kind: 'path', path: [{ field: 'quantity' }], total: 'rows' }),
            column('run', {
                kind: 'formula',
                formula: formula({ op: 'runningSum', args: [{ col: 'amount' }] }),
            }),
            column('share', {
                kind: 'formula',
                formula: formula({ op: 'div', args: [{ col: 'run' }, { value: 2 }] }),
            }),
            column('teamName', {
                kind: 'formula',
                formula: formula({ record: 'byTeam', path: [{ field: 'name' }] }),
            }),
            column('total', {
                kind: 'formula',
                formula: formula({ op: 'sum', args: [{ path: [{ field: 'cost' }] }] }),
            }),
        ],
        groups: [
            level('byStatus', 'status'),
            level('byTeam', 'team'),
            level('byDay', 'day', 'month'),
        ],
        layout: [],
    }
}

describe('what a group row has', () => {
    const keys = (set: ReadonlySet<string>) => [...set].sort()

    it('finds running values and percents, and the columns that read them', () => {
        expect(keys(windowColumns(levelsDraft().columns))).toEqual(['run', 'share'])
    })

    it('reads on a level only totals, rows totals, its keys and outer records', () => {
        const d = levelsDraft()
        expect(keys(levelValueColumns(d, CATALOG, 0))).toEqual(['amount', 'qty', 'status', 'total'])
        expect(keys(levelValueColumns(d, CATALOG, 1))).toEqual([
            'amount',
            'qty',
            'status',
            'team',
            'teamName',
            'total',
        ])
        // A bucketed key is a month, not the day itself.
        expect(levelValueColumns(d, CATALOG, 2).has('day')).toBe(false)
    })

    it('offers a label only what its level has, and a sort anything but a running value', () => {
        const d = levelsDraft()
        const label = formulaContext(d, CATALOG, { kind: 'level', index: 0 })
        expect(label.values.map(v => v.key)).toEqual(['status', 'amount', 'qty', 'total'])
        expect(label.records).toEqual([])
        const inner = formulaContext(d, CATALOG, { kind: 'level', index: 1 })
        expect(inner.records.map(r => r.group)).toEqual(['byTeam'])
        const sort = formulaContext(d, CATALOG, { kind: 'level' })
        expect(sort.values.map(v => v.key)).not.toContain('run')
        expect(sort.values.map(v => v.key)).not.toContain('share')
        expect(sort.values.map(v => v.key)).toContain('customer')
    })

    it('adds the rows shown only for numbers and amounts that are not running values', () => {
        const d = levelsDraft()
        const choices = columnChoices(d, CATALOG)
        expect(keys(rowsTotalColumns(choices, d.columns))).toEqual(['amount', 'qty', 'total'])
    })

    // bind.go: child rows keep their own order; run.go notASortKey: a running value
    // or a percent is not a sort key. The menu offers neither.
    it('sorts by no child rows column and no running value', () => {
        const teams = CATALOG.get(TEAMS_KEY)
        if (!teams) throw new Error('fixture')
        const common = {
            aggregate: '' as const,
            scale: null,
            format: '',
            total: 'recompute' as const,
            explicitTotal: false,
            labelField: '',
        }
        const d: BuilderDraft = {
            ...emptyDraft(teams),
            columns: [
                { ...common, kind: 'path', id: 'name', label: 'Name', path: [{ field: 'name' }] },
                {
                    ...common,
                    kind: 'path',
                    id: 'count',
                    label: 'Count',
                    path: [{ link: 'sales', mode: 'aggregate' }, { field: 'id' }],
                    aggregate: 'count',
                },
                {
                    ...common,
                    kind: 'path',
                    id: 'buyer',
                    label: 'Buyer',
                    path: [{ link: 'sales', mode: 'nest' }, { field: 'customer' }],
                },
                {
                    ...common,
                    kind: 'formula',
                    id: 'run',
                    label: 'Run',
                    formula: {
                        kind: 'editable',
                        node: { op: 'runningSum', args: [{ col: 'count' }] },
                    },
                },
            ],
        }
        const choices = columnChoices(d, CATALOG)
        expect(sortColumns(choices, d.columns).map(c => c.key)).toEqual(['name', 'count'])
    })

    it('knows when a formula reads a column', () => {
        expect(readsColumn({ op: 'concat', args: [{ col: 'status' }] }, 'status')).toBe(true)
        expect(readsColumn({ op: 'concat', args: [{ col: 'team' }] }, 'status')).toBe(false)
    })
})

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

    // A row inside a section adds to it, so it cannot read it (layout.go subtotal).
    it('a row inside a section is not offered that section', () => {
        const inside = draft()
        inside.layout = inside.layout.map(item =>
            item.kind === 'row' ? { ...item, within: 'open' } : item
        )
        expect(formulaContext(inside, CATALOG, { kind: 'layout', index: 1 }).values).toEqual([])
    })

    // Ruling 7: a computed row's formula and its cells read params (layout.go param).
    it('a layout row reads the params', () => {
        const ctx = formulaContext(draft(), CATALOG, { kind: 'layout', index: 1 })
        expect(ctx.params.map(p => p.id)).toEqual(['period'])
    })

    // F16 / R21: a computed row may look a value up; the lookup reads params
    // (lookup.go lookupCtx.param).
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

// alias.go targetOf / nodeTarget: the compiler gives a formula of a link the
// record it points at, so the builder reads the same target (R2).
describe('the record a column points at', () => {
    const common = {
        aggregate: '' as const,
        scale: null,
        format: '',
        total: 'recompute' as const,
        explicitTotal: false,
        labelField: '',
    }
    const path = (id: string, hops: SpecHop[]): ColumnDraft => ({
        ...common,
        kind: 'path',
        id,
        label: id,
        path: hops,
    })
    const formula = (id: string, node: SpecNode): ColumnDraft => ({
        ...common,
        kind: 'formula',
        id,
        label: id,
        formula: { kind: 'editable', node },
    })
    const team: SpecNode = { path: [{ link: 'team' }] }
    const positive: SpecNode = {
        op: 'gt',
        args: [{ path: [{ field: 'quantity' }] }, { value: 0 }],
    }
    const level = (id: string, col: string, bucket: '' | 'month' = '') => ({
        id,
        col,
        bucket,
        subtotals: false,
        open: false,
        label: null,
        having: emptyGroup(),
        fill: { on: false, where: emptyGroup() },
    })
    function targets(): BuilderDraft {
        return {
            ...draft(),
            columns: [
                path('team', [{ link: 'team' }]),
                path('sale', [{ field: 'id' }]),
                path('day', [{ field: 'day' }]),
                formula('chosen', { op: 'if', args: [positive, { col: 'team' }, team] }),
                formula('first', {
                    op: 'case',
                    args: [positive, { value: null }, { col: 'chosen' }],
                }),
                formula('either', { op: 'coalesce', args: [{ value: null }, team] }),
                formula('looked', {
                    op: 'lookup',
                    source: TEAMS_KEY,
                    where: { value: true },
                    value: { path: [{ field: 'id' }] },
                }),
                formula('again', { record: 'byChosen', path: [{ field: 'id' }] }),
                formula('loop', { op: 'coalesce', args: [{ col: 'loop2' }] }),
                formula('loop2', { op: 'if', args: [positive, { col: 'loop' }, { value: null }] }),
                formula('name', { op: 'if', args: [positive, { value: 'a' }, { value: 'b' }] }),
                path('amount', [{ field: 'amount' }]),
            ],
            groups: [level('byChosen', 'chosen'), level('byDay', 'day', 'month')],
        }
    }

    it('reads the record of a path, an if, a case, a coalesce, a lookup and a group record', () => {
        const d = targets()
        const at = (id: string) => columnTarget(d, CATALOG, id)?.key
        expect(at('team')).toBe(TEAMS_KEY)
        expect(at('sale')).toBe(SALES_KEY)
        expect(at('chosen')).toBe(TEAMS_KEY)
        expect(at('first')).toBe(TEAMS_KEY)
        expect(at('either')).toBe(TEAMS_KEY)
        expect(at('looked')).toBe(TEAMS_KEY)
        expect(at('again')).toBe(TEAMS_KEY)
    })

    it('reads none for a value, a cycle or an unknown column', () => {
        const d = targets()
        const at = (id: string) => columnTarget(d, CATALOG, id)
        expect(at('name')).toBeUndefined()
        expect(at('amount')).toBeUndefined()
        expect(at('day')).toBeUndefined()
        expect(at('loop')).toBeUndefined()
        expect(at('gone')).toBeUndefined()
    })

    // The Trial Balance groups by if(prior year, retained earnings, account): the
    // level carries the account, its fill and labels read it, and "Show as" applies.
    it('a group by a formula of a link carries its record everywhere the builder reads it', () => {
        const d = targets()
        expect(recordGroups(d, CATALOG)).toEqual([
            { group: 'byChosen', label: 'chosen', source: TEAMS_KEY },
        ])
        expect(groupTarget(d, CATALOG, { col: 'chosen', bucket: '' })?.key).toBe(TEAMS_KEY)
        // A bucketed date has no record (alias.go findTargets).
        expect(groupTarget(d, CATALOG, { col: 'day', bucket: 'month' })).toBeUndefined()
        const scope = levelScope(d, CATALOG, 0)
        expect(scope.columns.find(c => c.key === 'chosen')?.target?.key).toBe(TEAMS_KEY)
        expect(scope.records).toEqual([{ group: 'byChosen', label: 'chosen', source: TEAMS_KEY }])
        const chosen = d.columns.find(c => c.id === 'chosen')
        if (!chosen) throw new Error('fixture')
        expect(linkTarget(d, CATALOG, chosen)?.key).toBe(TEAMS_KEY)
        expect(linkTarget(d, CATALOG, { ...chosen, aggregate: 'count' })).toBeUndefined()
        const ctx = formulaContext(d, CATALOG, { kind: 'level', index: 0 })
        expect(ctx.records.map(r => r.source)).toEqual([TEAMS_KEY])
    })

    it('a link column shows its record, a field or a total does not', () => {
        const d = targets()
        const column = (hops: SpecHop[], aggregate: 'count' | '' = '') => ({
            ...path('x', hops),
            aggregate,
        })
        expect(linkTarget(d, CATALOG, column([{ link: 'team' }]))?.key).toBe(TEAMS_KEY)
        expect(linkTarget(d, CATALOG, column([{ field: 'id' }]))?.key).toBe(SALES_KEY)
        expect(linkTarget(d, CATALOG, column([{ field: 'customer' }]))).toBeUndefined()
        expect(linkTarget(d, CATALOG, column([{ link: 'team' }], 'count'))).toBeUndefined()
        // Through a to-many hop a cell is many records, not one (alias.go pathTarget).
        expect(
            linkTarget(d, CATALOG, column([{ link: 'team' }, { link: 'sales' }, { field: 'id' }]))
        ).toBeUndefined()
    })
})

describe('a percent of a section', () => {
    const share: ColumnDraft = {
        kind: 'formula',
        id: 'share',
        label: 'Share',
        formula: {
            kind: 'editable',
            node: { op: 'percentOfSection', args: [{ col: 'amount' }], subtotal: 'open', scale: 1 },
        },
        aggregate: '',
        scale: null,
        format: '',
        total: 'recompute',
        explicitTotal: false,
        labelField: '',
    }
    const withShare = (): BuilderDraft => {
        const d = draft()
        return { ...d, columns: [...d.columns, share] }
    }

    it("a column formula knows the layout's sections", () => {
        const ctx = formulaContext(withShare(), CATALOG, { kind: 'column', selfKey: 'share' })
        expect(ctx.sections).toEqual([{ id: 'open', title: 'Open' }])
        expect(formulaContext(withShare(), CATALOG, { kind: 'layout', index: 1 }).sections).toEqual(
            [{ id: 'open', title: 'Open' }]
        )
    })

    // Its cells are filled after the statements run (R2): another formula would read none.
    it('is read by no other formula, and is a percent column no total adds', () => {
        const d = withShare()
        expect(sectionPercentColumns(d.columns)).toEqual(new Set(['share']))
        for (const place of [
            { kind: 'detail' as const },
            { kind: 'column' as const, selfKey: 'team' },
        ]) {
            expect(formulaContext(d, CATALOG, place).values.map(v => v.key)).not.toContain('share')
        }
        expect(windowColumns(d.columns).has('share')).toBe(true)
        expect(rowsTotalColumns(columnChoices(d, CATALOG), d.columns).has('share')).toBe(false)
    })

    it("stays out of a computed row's values", () => {
        const ctx = formulaContext(withShare(), CATALOG, { kind: 'layout', index: 1 })
        expect(ctx.values.map(v => v.key)).toEqual(['open'])
    })
})
