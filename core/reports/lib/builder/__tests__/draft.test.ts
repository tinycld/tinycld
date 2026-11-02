import { describe, expect, it } from 'vitest'
import { CURRENT_SPEC_VERSION, type ReportSpec } from '../../spec'
import {
    builderSchema,
    type ColumnDraft,
    draftFromSpec,
    emptyDraft,
    type FormulaDraft,
    specFromDraft,
} from '../draft'
import { CATALOG, SALES_KEY, TEAMS_KEY } from './catalog-fixture'

const RICH: ReportSpec = {
    version: CURRENT_SPEC_VERSION,
    source: SALES_KEY,
    params: [{ id: 'period', type: 'dateRange', label: 'Day', default: { preset: 'thisMonth' } }],
    columns: [
        { id: 'status', label: 'Status', path: [{ field: 'status' }] },
        { id: 'team', label: 'Team › Name', path: [{ link: 'team' }, { field: 'name' }] },
        { id: 'amount', label: 'Amount', path: [{ field: 'amount' }], aggregate: 'sum' },
        { id: 'cost', label: 'Cost', path: [{ field: 'cost' }], aggregate: 'sum' },
        {
            id: 'profit',
            label: 'Profit',
            formula: { op: 'sub', args: [{ col: 'amount' }, { col: 'cost' }] },
            aggregate: 'sum',
        },
        { id: 'avg', label: 'Average', path: [{ field: 'amount' }], aggregate: 'avg', scale: 0 },
    ],
    filter: { op: 'between', args: [{ path: [{ field: 'day' }] }, { param: 'period' }] },
    sort: [{ col: 'amount', dir: 'desc' }],
    groups: [{ id: 'byStatus', col: 'status', subtotals: true }],
    pivot: { path: [{ field: 'day' }], bucket: 'month', cell: 'amount', maxColumns: 12 },
    layout: [
        { section: 'open', title: 'Open', key: 'open' },
        { section: 'paid', title: 'Paid', key: 'paid' },
        {
            row: 'gap',
            label: 'Open less paid',
            formula: { op: 'sub', args: [{ subtotal: 'open' }, { subtotal: 'paid' }] },
        },
    ],
}

describe('draftFromSpec and specFromDraft', () => {
    it('round-trip a spec the builder can edit in full', () => {
        const draft = draftFromSpec(RICH, { title: 'Sales', description: '' }, CATALOG)
        expect(specFromDraft(draft, CATALOG)).toEqual(RICH)
    })

    it('keeps parts the builder cannot edit unchanged', () => {
        const shipped: ReportSpec = {
            version: CURRENT_SPEC_VERSION,
            source: SALES_KEY,
            columns: [
                { id: 'customer', label: 'Customer', path: [{ field: 'customer' }] },
                {
                    id: 'flag',
                    label: 'Big',
                    formula: {
                        op: 'if',
                        args: [
                            { op: 'gt', args: [{ col: 'amount' }, { value: 100 }] },
                            { value: 1 },
                            { value: 0 },
                        ],
                    },
                },
                { id: 'amount', label: 'Amount', path: [{ field: 'amount' }] },
            ],
            filter: { op: 'case', args: [{ value: true }, { value: true }] },
        }
        const draft = draftFromSpec(shipped, { title: 'Shipped', description: '' }, CATALOG)
        expect(draft.columns[1]).toMatchObject({ kind: 'formula', formula: { kind: 'fixed' } })
        const renamed = { ...draft, title: 'Renamed' }
        expect(specFromDraft(renamed, CATALOG)).toEqual(shipped)
    })

    it('keeps a formula over a column id the editor cannot read', () => {
        const shipped: ReportSpec = {
            version: CURRENT_SPEC_VERSION,
            source: SALES_KEY,
            columns: [
                { id: 'netAmount', label: 'Net', path: [{ field: 'amount' }], aggregate: 'sum' },
                {
                    id: 'half',
                    label: 'Half',
                    formula: { op: 'div', args: [{ col: 'netAmount' }, { value: 2 }], scale: 0 },
                },
            ],
        }
        const draft = draftFromSpec(shipped, { title: 'Shipped', description: '' }, CATALOG)
        expect(draft.columns[1]).toMatchObject({ kind: 'formula', formula: { kind: 'fixed' } })
        expect(specFromDraft(draft, CATALOG)).toEqual(shipped)
    })

    it('drops a param no filter or formula asks for', () => {
        const draft = draftFromSpec(RICH, { title: 'Sales', description: '' }, CATALOG)
        const unasked = {
            ...draft,
            filter: { kind: 'group' as const, match: 'all' as const, items: [] },
        }
        expect(specFromDraft(unasked, CATALOG).params).toBeUndefined()
    })
})

// The engine keeps whole cents when an amount is divided by a number: such a
// division must say scale 0 (formula.go), so the builder must know which
// columns and subtotals hold amounts.
describe('the scale of a division and an average', () => {
    const META = { title: 'Sales', description: '' }
    const amountCols: ReportSpec['columns'] = [
        { id: 'status', label: 'Status', path: [{ field: 'status' }] },
        { id: 'amount', label: 'Amount', path: [{ field: 'amount' }], aggregate: 'sum' },
        { id: 'cost', label: 'Cost', path: [{ field: 'cost' }], aggregate: 'sum' },
        {
            id: 'profit',
            label: 'Profit',
            formula: { op: 'sub', args: [{ col: 'amount' }, { col: 'cost' }] },
        },
    ]

    it('divides an amount formula by a number in whole cents', () => {
        const draft = draftFromSpec(
            { version: 1, source: SALES_KEY, columns: amountCols },
            META,
            CATALOG
        )
        const half: ColumnDraft = {
            kind: 'formula',
            id: 'half',
            label: 'Half',
            aggregate: '',
            scale: null,
            format: '',
            formula: { kind: 'editable', expr: { div: [{ var: 'profit' }, 2] } },
        }
        const spec = specFromDraft({ ...draft, columns: [...draft.columns, half] }, CATALOG)
        expect(spec.columns[4]?.formula).toEqual({
            op: 'div',
            args: [{ col: 'profit' }, { value: 2 }],
            scale: 0,
        })
    })

    it('averages an amount formula in whole cents', () => {
        const draft = draftFromSpec(
            { version: 1, source: SALES_KEY, columns: amountCols },
            META,
            CATALOG
        )
        const columns = draft.columns.map(column =>
            column.id === 'profit' ? { ...column, aggregate: 'avg' as const } : column
        )
        expect(specFromDraft({ ...draft, columns }, CATALOG).columns[3]).toMatchObject({
            aggregate: 'avg',
            scale: 0,
        })
    })

    const layoutSpec = (columns: ReportSpec['columns']): ReportSpec => ({
        version: CURRENT_SPEC_VERSION,
        source: SALES_KEY,
        columns,
        groups: [{ id: 'byStatus', col: 'status', subtotals: true }],
        layout: [
            { section: 'open', title: 'Open', key: 'open' },
            { row: 'third', label: 'A third', formula: { value: 0 } },
        ],
    })
    const third: FormulaDraft = { kind: 'editable', expr: { div: [{ var: 'open' }, 3] } }
    const thirdOfOpen = (spec: ReportSpec) => {
        const draft = draftFromSpec(spec, META, CATALOG)
        const layout = draft.layout.map(item =>
            item.kind === 'row' ? { ...item, formula: third } : item
        )
        return specFromDraft({ ...draft, layout }, CATALOG).layout?.[1]?.formula
    }

    it('divides a layout subtotal of counts at the ratio scale', () => {
        const counts: ReportSpec['columns'] = [
            { id: 'status', label: 'Status', path: [{ field: 'status' }] },
            { id: 'sales', label: 'Sales', path: [{ field: 'id' }], aggregate: 'count' },
        ]
        expect(thirdOfOpen(layoutSpec(counts))).toEqual({
            op: 'div',
            args: [{ subtotal: 'open' }, { value: 3 }],
            scale: 4,
        })
    })

    it('divides a layout subtotal of amounts in whole cents', () => {
        expect(thirdOfOpen(layoutSpec(amountCols))).toEqual({
            op: 'div',
            args: [{ subtotal: 'open' }, { value: 3 }],
            scale: 0,
        })
    })
})

describe('emptyDraft', () => {
    it('starts a new report with the source’s first field', () => {
        const source = CATALOG.get(TEAMS_KEY)
        if (!source) throw new Error('fixture')
        const draft = emptyDraft(source)
        expect(draft.columns).toEqual([
            {
                kind: 'path',
                id: 'name',
                label: 'Name',
                path: [{ field: 'name' }],
                aggregate: '',
                scale: null,
                format: '',
            },
        ])
        expect(specFromDraft({ ...draft, title: 'Teams' }, CATALOG)).toEqual({
            version: CURRENT_SPEC_VERSION,
            source: TEAMS_KEY,
            columns: [{ id: 'name', label: 'Name', path: [{ field: 'name' }] }],
        })
    })
})

describe('builderSchema', () => {
    const schema = builderSchema(CATALOG)
    const base = draftFromSpec(RICH, { title: 'Sales', description: '' }, CATALOG)
    const messages = (value: unknown) => {
        const parsed = schema.safeParse(value)
        return parsed.success
            ? []
            : parsed.error.issues.map(issue => `${issue.path.join('.')}: ${issue.message}`)
    }

    it('accepts a valid draft', () => {
        expect(messages(base)).toEqual([])
    })

    it('asks how to total a column across a to-many link', () => {
        const source = CATALOG.get(TEAMS_KEY)
        if (!source) throw new Error('fixture')
        const draft = {
            ...emptyDraft(source),
            title: 'Teams',
            columns: [
                {
                    kind: 'path',
                    id: 'n',
                    label: 'Sales',
                    path: [{ link: 'sales', mode: 'aggregate' }, { field: 'id' }],
                    aggregate: '',
                    scale: null,
                    format: '',
                },
            ],
        }
        expect(messages(draft)).toEqual(['columns.0.aggregate: Choose how to total it'])
    })

    it('refuses a pivot or a layout without a group, and two columns with one id', () => {
        const noGroups = { ...base, groups: [], sort: [] }
        expect(messages(noGroups)).toEqual([
            'pivot.on: A pivot needs a group',
            'layout: A layout needs a group',
        ])
        const twice = { ...base, columns: [...base.columns, { ...base.columns[0] }] }
        expect(messages(twice)).toContain('columns.6.id: Two columns have this id')
    })

    it('needs a title', () => {
        expect(messages({ ...base, title: ' ' })).toEqual(['title: Enter a title'])
    })
})
