import { describe, expect, it } from 'vitest'
import { CURRENT_SPEC_VERSION, type ParamDef, type ReportSpec, type SpecNode } from '../../spec'
import { MAX_COLUMNS } from '../columns'
import {
    type BuilderDraft,
    builderSchema,
    type ColumnDraft,
    draftFromSpec,
    draftIssues,
    emptyDraft,
    type FormulaDraft,
    groupTarget,
    keptParams,
    labelStart,
    levelScope,
    offersDetailBlank,
    offersHaving,
    showsDetailLabel,
    showsHaving,
    specFromDraft,
} from '../draft'
import { emptyCondition, emptyGroup } from '../filter'
import { formulaContext } from '../formula-context'
import { describeNode } from '../formula-describe'
import { isEditableNode } from '../formula-ops'
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
                // Outside a lookup an exists belongs to the filter (formula.go ctxBase.exists).
                {
                    id: 'flag',
                    label: 'Has a team',
                    formula: { op: 'exists', link: [{ link: 'team' }] },
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

    it('edits a formula over any column id', () => {
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
        expect(draft.columns[1]).toMatchObject({ kind: 'formula', formula: { kind: 'editable' } })
        expect(specFromDraft(draft, CATALOG)).toEqual(shipped)
    })

    it('edits an if, a lookup and a group record, and saves them as they were read', () => {
        const spec: ReportSpec = {
            version: CURRENT_SPEC_VERSION,
            source: SALES_KEY,
            columns: [
                { id: 'team', label: 'Team', path: [{ link: 'team' }] },
                { id: 'amount', label: 'Amount', path: [{ field: 'amount' }], aggregate: 'sum' },
                { id: 'cost', label: 'Cost', path: [{ field: 'cost' }], aggregate: 'sum' },
                {
                    id: 'larger',
                    label: 'Larger',
                    formula: {
                        op: 'if',
                        args: [
                            { op: 'gt', args: [{ col: 'amount' }, { col: 'cost' }] },
                            { col: 'amount' },
                            { col: 'cost' },
                        ],
                    },
                },
                {
                    id: 'north',
                    label: 'North',
                    formula: {
                        op: 'lookup',
                        source: TEAMS_KEY,
                        where: {
                            op: 'eq',
                            args: [{ path: [{ field: 'name' }] }, { value: 'North' }],
                        },
                        value: { path: [{ field: 'name' }] },
                        pick: 'min',
                    },
                },
                {
                    id: 'name',
                    label: 'Name',
                    formula: { record: 'byTeam', path: [{ field: 'name' }] },
                },
            ],
            groups: [{ id: 'byTeam', col: 'team' }],
        }
        const draft = draftFromSpec(spec, { title: 'T', description: '' }, CATALOG)
        for (const column of draft.columns.slice(3)) {
            expect(column).toMatchObject({ kind: 'formula', formula: { kind: 'editable' } })
        }
        expect(specFromDraft(draft, CATALOG)).toEqual(spec)
    })

    it('keeps a param a formula reads, in a column or a layout row lookup', () => {
        const spec: ReportSpec = {
            version: CURRENT_SPEC_VERSION,
            source: SALES_KEY,
            params: [
                { id: 'min', type: 'number', label: 'Least' },
                { id: 'team', type: 'text', label: 'Team' },
            ],
            columns: [
                { id: 'status', label: 'Status', path: [{ field: 'status' }] },
                {
                    id: 'over',
                    label: 'Over',
                    formula: { op: 'gt', args: [{ col: 'status' }, { param: 'min' }] },
                },
            ],
            groups: [{ id: 'byStatus', col: 'status' }],
            layout: [
                { section: 'open', title: 'Open', key: 'open' },
                {
                    row: 'named',
                    label: 'Named',
                    formula: {
                        op: 'lookup',
                        source: TEAMS_KEY,
                        where: {
                            op: 'eq',
                            args: [{ path: [{ field: 'name' }] }, { param: 'team' }],
                        },
                        value: { op: 'count', args: [{ path: [{ field: 'id' }] }] },
                    },
                },
            ],
        }
        const draft = draftFromSpec(spec, { title: 'T', description: '' }, CATALOG)
        expect(specFromDraft(draft, CATALOG).params).toEqual(spec.params)
    })

    it('saves a lookup with no where, and paths whose links keep only rows with them, as read', () => {
        const spec: ReportSpec = {
            version: CURRENT_SPEC_VERSION,
            source: SALES_KEY,
            columns: [
                { id: 'team', label: 'Team', path: [{ link: 'team' }] },
                {
                    id: 'first_team',
                    label: 'First team',
                    formula: {
                        op: 'lookup',
                        source: TEAMS_KEY,
                        value: { path: [{ field: 'name' }] },
                    },
                },
                {
                    id: 'team_name',
                    label: 'Team name',
                    formula: { path: [{ link: 'team', with: 'required' }, { field: 'name' }] },
                },
                {
                    id: 'group_team',
                    label: 'Group team',
                    formula: {
                        record: 'byTeam',
                        path: [{ field: 'name' }],
                    },
                },
                {
                    id: 'kept',
                    label: 'Kept',
                    formula: { path: [{ link: 'team', with: 'optional' }] },
                },
            ],
            groups: [{ id: 'byTeam', col: 'team' }],
        }
        const draft = draftFromSpec(spec, { title: 'T', description: '' }, CATALOG)
        for (const column of draft.columns.slice(1)) {
            expect(column).toMatchObject({ kind: 'formula', formula: { kind: 'editable' } })
        }
        expect(specFromDraft(draft, CATALOG)).toEqual(spec)
    })

    it('saves no format a formula column no longer shows', () => {
        const spec: ReportSpec = {
            version: CURRENT_SPEC_VERSION,
            source: SALES_KEY,
            columns: [
                { id: 'customer', label: 'Customer', path: [{ field: 'customer' }] },
                {
                    id: 'ratio',
                    label: 'Ratio',
                    formula: { op: 'div', args: [{ value: 1 }, { value: 4 }], scale: 4 },
                    format: 'percent',
                },
            ],
        }
        const draft = draftFromSpec(spec, { title: 'T', description: '' }, CATALOG)
        expect(specFromDraft(draft, CATALOG)).toEqual(spec)
        const columns = draft.columns.map(column =>
            column.kind === 'formula'
                ? { ...column, formula: { kind: 'editable' as const, node: { value: 'text' } } }
                : column
        )
        expect(specFromDraft({ ...draft, columns }, CATALOG).columns[1]).toEqual({
            id: 'ratio',
            label: 'Ratio',
            formula: { value: 'text' },
        })
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
// columns and subtotals hold amounts. The editor settles scales as it edits; the
// spec settles them again, since a column read can become an amount elsewhere.
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
            total: 'recompute',
            explicitTotal: false,
            labelField: '',
            formula: {
                kind: 'editable',
                node: { op: 'div', args: [{ col: 'profit' }, { value: 2 }] },
            },
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
    const third: FormulaDraft = {
        kind: 'editable',
        node: { op: 'div', args: [{ subtotal: 'open' }, { value: 3 }] },
    }
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
                total: 'recompute',
                explicitTotal: false,
                labelField: '',
                detail: 'show',
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
                    total: 'recompute',
                    explicitTotal: false,
                    labelField: '',
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

const V2: ReportSpec = {
    version: 2,
    source: SALES_KEY,
    params: [
        { id: 'asOf', type: 'asOf', label: 'As of', default: { preset: 'endOfLastMonth' } },
        {
            id: 'rows',
            type: 'choice',
            label: 'Rows',
            default: 'nonzero',
            options: [
                { value: 'nonzero', label: 'Non-zero' },
                { value: 'all', label: 'All' },
            ],
        },
        { id: 'min', type: 'number', label: 'Quantity' },
        {
            id: 'teams',
            type: 'choice',
            label: 'Teams',
            optionsFrom: {
                source: TEAMS_KEY,
                value: [{ field: 'id' }],
                label: [{ field: 'name' }],
            },
            multiple: true,
        },
    ],
    columns: [
        { id: 'team', label: 'Team', path: [{ link: 'team' }] },
        { id: 'name', label: 'Name', formula: { record: 'byTeam', path: [{ field: 'name' }] } },
        {
            id: 'amount',
            label: 'Amount',
            path: [{ field: 'amount' }],
            aggregate: 'sum',
            total: 'rows',
        },
        { id: 'quantity', label: 'Quantity', path: [{ field: 'quantity' }], aggregate: 'sum' },
        {
            id: 'north',
            label: 'North',
            formula: {
                op: 'lookup',
                source: TEAMS_KEY,
                where: { op: 'eq', args: [{ path: [{ field: 'name' }] }, { value: 'North' }] },
                value: { path: [{ field: 'id' }] },
                pick: 'min',
            },
        },
    ],
    filter: {
        op: 'and',
        args: [
            {
                op: 'lte',
                args: [
                    { op: 'addDays', args: [{ path: [{ field: 'day' }] }, { value: 0 }] },
                    { param: 'asOf' },
                ],
            },
            { op: 'in', args: [{ path: [{ link: 'team' }] }, { param: 'teams' }] },
        ],
    },
    sort: [
        { formula: { record: 'byTeam', path: [{ field: 'name' }] }, dir: 'desc' },
        { col: 'amount' },
    ],
    groups: [
        {
            id: 'byTeam',
            col: 'team',
            label: { record: 'byTeam', path: [{ field: 'name' }] },
            having: {
                op: 'and',
                args: [
                    { op: 'gte', args: [{ col: 'quantity' }, { param: 'min' }] },
                    {
                        op: 'or',
                        args: [
                            { op: 'eq', args: [{ param: 'rows' }, { value: 'all' }] },
                            {
                                op: 'ne',
                                args: [
                                    { op: 'sum', args: [{ path: [{ field: 'amount' }] }] },
                                    { value: 0 },
                                ],
                            },
                        ],
                    },
                ],
            },
            fill: {
                where: { op: 'startsWith', args: [{ path: [{ field: 'name' }] }, { value: 'A' }] },
            },
            open: true,
        },
    ],
    detail: {
        label: {
            op: 'concat',
            args: [
                { path: [{ field: 'customer' }] },
                { value: ' · ' },
                { path: [{ field: 'day' }] },
            ],
        },
    },
    display: {
        layouts: [
            {
                width: 'mobile',
                tracks: 3,
                cells: [
                    { col: 'team', line: 1, span: 3 },
                    { col: 'amount', line: 2, justify: 'end' },
                ],
            },
        ],
    },
}

const META = { title: 'T', description: '' }

describe('spec version 2 in the builder', () => {
    it('keeps every v2 part through the draft', () => {
        const draft = draftFromSpec(V2, META, CATALOG)
        expect(builderSchema(CATALOG).safeParse(draft).success).toBe(true)
        expect(specFromDraft(draft, CATALOG)).toEqual(V2)
        // A save that changes nothing writes the same bytes.
        expect(JSON.stringify(specFromDraft(draft, CATALOG))).toBe(JSON.stringify(V2))
    })

    it('reads an aggregate condition and a relation condition as editable, and the rest as fixed', () => {
        const draft = draftFromSpec(V2, META, CATALOG)
        const [column, fixed] = draft.groups[0]?.having.items ?? []
        expect(column).toMatchObject({
            kind: 'condition',
            subject: 'column',
            col: 'quantity',
            op: 'gte',
            param: 'min',
        })
        // or(...) reads back as an Any-of group: the param test is fixed, the total a condition (F8).
        expect(fixed).toMatchObject({
            kind: 'group',
            match: 'any',
            items: [
                { kind: 'fixed' },
                { kind: 'condition', subject: 'total', aggregate: 'sum', op: 'ne' },
            ],
        })
        expect(draft.filter.items[1]).toMatchObject({
            kind: 'condition',
            subject: 'field',
            path: [{ link: 'team' }],
            op: 'in',
            param: 'teams',
        })
        expect(draft.groups[0]?.fill).toMatchObject({
            on: true,
            where: { items: [{ kind: 'condition', op: 'startsWith' }] },
        })
        expect(draft.sort[0]).toMatchObject({ kind: 'formula', dir: 'desc' })
        expect(draft.columns[2]).toMatchObject({ total: 'rows' })
    })

    it('drops a param only when nothing reads it any more', () => {
        const draft = draftFromSpec(V2, META, CATALOG)
        const noHaving = {
            ...draft,
            groups: draft.groups.map(g => ({ ...g, having: emptyGroup() })),
        }
        expect(keptParams(noHaving).map(p => p.id)).toEqual(['asOf', 'teams'])
        expect(specFromDraft(noHaving, CATALOG).params?.map(p => p.id)).toEqual(['asOf', 'teams'])
        // A newly asked param joins the kept ones.
        const asked: ParamDef = { id: 'least', type: 'amount', label: 'Least' }
        expect(keptParams(noHaving, asked).map(p => p.id)).toEqual(['asOf', 'teams', 'least'])
        // A param a sort formula, a group label or a row label reads stays.
        const read = (node: SpecNode) => ({ kind: 'editable' as const, node })
        const readers = {
            ...draft,
            filter: emptyGroup(),
            groups: draft.groups.map(g => ({
                ...g,
                having: emptyGroup(),
                label: read({ param: 'rows' }),
            })),
            sort: [
                {
                    kind: 'formula' as const,
                    formula: read({ param: 'min' }),
                    dir: 'asc' as const,
                    explicitDir: false,
                },
            ],
            detailLabel: read({ param: 'asOf' }),
        }
        expect(keptParams(readers).map(p => p.id)).toEqual(['asOf', 'rows', 'min'])
    })

    it('keeps the label field of a link column and of a pivot', () => {
        const spec: ReportSpec = {
            version: 2,
            source: SALES_KEY,
            columns: [
                { id: 'team', label: 'Team', path: [{ link: 'team' }], labelField: 'region' },
                { id: 'plain', label: 'Plain', path: [{ link: 'team' }] },
                { id: 'amount', label: 'Amount', path: [{ field: 'amount' }], aggregate: 'sum' },
            ],
            groups: [{ id: 'byTeam', col: 'team' }],
            pivot: {
                path: [{ link: 'team' }],
                cell: 'amount',
                maxColumns: 50,
                labelField: 'region',
            },
        }
        const draft = draftFromSpec(spec, META, CATALOG)
        expect(draft.columns[0]).toMatchObject({ labelField: 'region' })
        expect(draft.columns[1]).toMatchObject({ labelField: '' })
        expect(draft.pivot).toMatchObject({ labelField: 'region' })
        expect(JSON.stringify(specFromDraft(draft, CATALOG))).toBe(JSON.stringify(spec))
    })

    it('reads a fill condition that matches the group above, and writes it back', () => {
        const spec: ReportSpec = {
            version: 2,
            source: SALES_KEY,
            columns: [
                { id: 'region', label: 'Region', path: [{ link: 'team' }, { field: 'region' }] },
                { id: 'team', label: 'Team', path: [{ link: 'team' }] },
                { id: 'amount', label: 'Amount', path: [{ field: 'amount' }], aggregate: 'sum' },
            ],
            groups: [
                { id: 'g0', col: 'region' },
                {
                    id: 'g1',
                    col: 'team',
                    fill: {
                        where: {
                            op: 'eq',
                            args: [{ path: [{ field: 'region' }] }, { col: 'region' }],
                        },
                    },
                },
            ],
        }
        const draft = draftFromSpec(spec, META, CATALOG)
        expect(draft.groups[1]?.fill.where.items[0]).toMatchObject({
            kind: 'condition',
            op: 'eq',
            above: 'g0',
            value: '',
            param: '',
        })
        expect(builderSchema(CATALOG).safeParse(draft).success).toBe(true)
        expect(specFromDraft(draft, CATALOG)).toEqual(spec)
    })

    it('saves a fill with no conditions as an empty fill', () => {
        const spec: ReportSpec = {
            version: 2,
            source: SALES_KEY,
            columns: [{ id: 'team', label: 'Team', path: [{ link: 'team' }] }],
            groups: [{ id: 'byTeam', col: 'team', fill: {} }],
        }
        const draft = draftFromSpec(spec, META, CATALOG)
        expect(draft.groups[0]?.fill).toEqual({ on: true, where: emptyGroup() })
        expect(specFromDraft(draft, CATALOG)).toEqual(spec)
    })

    // The Trial Balance shape: a group by if(prior year, retained earnings, account)
    // carries the account, so its fill conditions read the linked records.
    it('reads a group by a formula of a link as a record group, and writes it back', () => {
        const positive: SpecNode = {
            op: 'gt',
            args: [{ path: [{ field: 'quantity' }] }, { value: 0 }],
        }
        const spec: ReportSpec = {
            version: 2,
            source: SALES_KEY,
            columns: [
                { id: 'team', label: 'Team', path: [{ link: 'team' }] },
                {
                    id: 'chosen',
                    label: 'Chosen',
                    formula: {
                        op: 'if',
                        args: [positive, { col: 'team' }, { path: [{ link: 'team' }] }],
                    },
                    labelField: 'region',
                },
                {
                    id: 'name',
                    label: 'Name',
                    formula: { record: 'byChosen', path: [{ field: 'name' }] },
                },
                { id: 'amount', label: 'Amount', path: [{ field: 'amount' }], aggregate: 'sum' },
            ],
            groups: [
                {
                    id: 'byChosen',
                    col: 'chosen',
                    label: { record: 'byChosen', path: [{ field: 'name' }] },
                    fill: {
                        where: {
                            op: 'startsWith',
                            args: [{ path: [{ field: 'name' }] }, { value: 'A' }],
                        },
                    },
                },
            ],
        }
        const draft = draftFromSpec(spec, META, CATALOG)
        expect(groupTarget(draft, CATALOG, { col: 'chosen' })?.key).toBe(TEAMS_KEY)
        expect(draft.groups[0]?.fill.where.items[0]).toMatchObject({
            kind: 'condition',
            subject: 'field',
            path: [{ field: 'name' }],
            op: 'startsWith',
            value: 'A',
        })
        expect(builderSchema(CATALOG).safeParse(draft).success).toBe(true)
        expect(JSON.stringify(specFromDraft(draft, CATALOG))).toBe(JSON.stringify(spec))
    })

    // levels.go: a bucketed level has no one value of its date column, so a label
    // formula starts from the bucket's text spelled by the first date's parts
    // (rows.go bucketSQL), never from {col}, which would show "(missing)" and run
    // as "(none)".
    it('starts a group label formula from what labels the level by default', () => {
        const columns: ColumnDraft[] = [
            {
                kind: 'path',
                id: 'day',
                label: 'Day',
                path: [{ field: 'day' }],
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
        ]
        const first: SpecNode = { op: 'min', args: [{ path: [{ field: 'day' }] }] }
        const year: SpecNode = { op: 'year', args: [first] }
        const month: SpecNode = { op: 'month', args: [first] }
        const padded: SpecNode = {
            op: 'if',
            args: [{ op: 'lt', args: [month, { value: 10 }] }, { value: '0' }, { value: '' }],
        }
        const concat = (...args: SpecNode[]) => ({ op: 'concat', args })
        const start = (bucket: '' | 'day' | 'month' | 'quarter' | 'year' | 'fiscalYear') =>
            labelStart({ id: 'g', col: 'day', bucket }, undefined, columns).node
        expect(start('year')).toEqual(concat(year))
        expect(start('fiscalYear')).toEqual(concat({ op: 'fiscalYear', args: [first] }))
        expect(start('quarter')).toEqual(
            concat(year, { value: '-Q' }, { op: 'quarter', args: [first] })
        )
        expect(start('month')).toEqual(concat(year, { value: '-' }, padded, month))
        expect(start('day')).toEqual(first)
        expect(start('')).toEqual(concat({ col: 'day' }))
        const teams = CATALOG.get(TEAMS_KEY)
        expect(labelStart({ id: 'g', col: 'team', bucket: '' }, teams, columns).node).toEqual(
            concat({ record: 'g', path: [{ field: 'name' }] })
        )
        // A level label reads every part of the bucketed start.
        const sales = CATALOG.get(SALES_KEY)
        if (!sales) throw new Error('fixture')
        const draft = { ...emptyDraft(sales), columns }
        const level = {
            ...draft,
            groups: [
                {
                    id: 'g',
                    col: 'day',
                    bucket: 'month' as const,
                    subtotals: false,
                    open: false,
                    label: null,
                    having: emptyGroup(),
                    fill: { on: false, where: emptyGroup() },
                },
            ],
        }
        const ctx = formulaContext(level, CATALOG, { kind: 'level', index: 0 })
        for (const bucket of ['day', 'month', 'quarter', 'year', 'fiscalYear'] as const) {
            const node = start(bucket)
            expect(isEditableNode(node, [])).toBe(true)
            expect(describeNode(ctx, node)).not.toContain('(missing)')
        }
    })

    it('reads a group level’s conditions against its columns and its record', () => {
        const draft = draftFromSpec(V2, META, CATALOG)
        expect(groupTarget(draft, CATALOG, { col: 'team' })?.key).toBe(TEAMS_KEY)
        expect(groupTarget(draft, CATALOG, { col: 'amount' })).toBeUndefined()
        const level = levelScope(draft, CATALOG, 0)
        const column = (key: string) => level.columns.find(c => c.key === key)
        expect(column('quantity')?.field.type).toBe('number')
        expect(column('gone')).toBeUndefined()
        // A link column is a relation to its target, so a condition asks for a record.
        expect(column('team')).toMatchObject({ label: 'Team', field: { type: 'relation' } })
        expect(column('team')?.target?.key).toBe(TEAMS_KEY)
        expect(level.records).toEqual([{ group: 'byTeam', label: 'Team', source: TEAMS_KEY }])
    })

    // levels.go own: a having reads only what a group row has.
    it('offers a level condition only the columns with a value on its rows', () => {
        const draft = draftFromSpec(V2, META, CATALOG)
        const customer: ColumnDraft = {
            kind: 'path',
            id: 'customer',
            label: 'Customer',
            path: [{ field: 'customer' }],
            aggregate: '',
            scale: null,
            format: '',
            total: 'recompute',
            explicitTotal: false,
            labelField: '',
        }
        const level = levelScope({ ...draft, columns: [...draft.columns, customer] }, CATALOG, 0)
        expect(level.columns.map(c => c.key)).not.toContain('customer')
        expect(level.columns.map(c => c.key)).toContain('quantity')
    })

    it('hides a layout first group’s having, and a pivot’s row label, unless one is set', () => {
        const draft = draftFromSpec(V2, META, CATALOG)
        const section = { kind: 'section' as const, id: 's', title: 'S', key: 'x' }
        const empty = { ...draft.groups[0], having: emptyGroup() }
        const laidOut = { layout: [section], groups: [empty] as BuilderDraft['groups'] }
        expect(offersHaving(laidOut, 0)).toBe(false)
        expect(showsHaving(laidOut, 0)).toBe(false)
        expect(offersHaving(laidOut, 1)).toBe(true)
        expect(showsHaving({ layout: [section], groups: draft.groups }, 0)).toBe(true)
        const pivot = { ...draft.pivot, on: true }
        expect(showsDetailLabel({ pivot, detailLabel: null })).toBe(false)
        const label = { kind: 'editable' as const, node: { value: 'x' } }
        expect(showsDetailLabel({ pivot, detailLabel: label })).toBe(true)
        expect(showsDetailLabel({ pivot: draft.pivot, detailLabel: null })).toBe(true)
    })

    it('refuses a having on a layout’s first group and a row label on a pivot', () => {
        const draft = draftFromSpec(V2, META, CATALOG)
        const section = { kind: 'section' as const, id: 's', title: 'S', key: 'x' }
        const laidOut = draftIssues({ ...draft, layout: [section] }, CATALOG)
        expect(laidOut).toContainEqual({
            path: ['groups', 0, 'having'],
            message: "A layout's sections are fixed rows: hide rows on a deeper group",
        })
        const label = { kind: 'editable' as const, node: { value: 'x' } }
        const pivoted = draftIssues(
            { ...draft, pivot: { ...draft.pivot, on: true }, detailLabel: label },
            CATALOG
        )
        expect(pivoted.map(i => i.path.join('.'))).toContain('detailLabel')
    })

    it('names a group condition the draft can no longer run', () => {
        const draft = draftFromSpec(V2, META, CATALOG)
        const columns = draft.columns.filter(c => c.id !== 'quantity')
        const issues = draftIssues({ ...draft, columns }, CATALOG).map(i => i.path.join('.'))
        expect(issues).toContain('groups.0.having')
        const fill = {
            on: true,
            where: {
                ...emptyGroup(),
                items: [{ ...emptyCondition([{ field: 'name' }]), op: 'eq' as const }],
            },
        }
        const blankFill = { ...draft, groups: draft.groups.map(g => ({ ...g, fill })) }
        expect(draftIssues(blankFill, CATALOG).map(i => i.path.join('.'))).toEqual([
            'groups.0.fill',
        ])
    })

    it('names a display layout the engine would refuse, at its cell', () => {
        const draft = draftFromSpec(V2, META, CATALOG)
        const mobile = { width: 'mobile' as const, tracks: 2, cells: [{ col: 'team', line: 1 }] }
        const tablet = {
            width: 'tablet' as const,
            tracks: 2,
            cells: [
                { col: 'team', line: 1, span: 2 },
                { col: 'amount', line: 1 },
            ],
        }
        const issues = draftIssues({ ...draft, display: { layouts: [mobile, tablet] } }, CATALOG)
        expect(issues).toEqual([
            { path: ['display', 'layouts', 1, 'cells', 1], message: 'no room on line 1' },
        ])
        const wide = { ...mobile, tracks: 13 }
        const empty = { ...mobile, width: 'desktop' as const, cells: [] }
        const refused = draftIssues({ ...draft, display: { layouts: [wide, empty] } }, CATALOG)
        expect(refused).toEqual([
            { path: ['display', 'layouts', 0, 'tracks'], message: 'tracks is 1 to 12' },
            {
                path: ['display', 'layouts', 1, 'cells'],
                message: 'a layout places at least one column',
            },
        ])
    })

    it('names a column an arrangement does not have or places twice, and a second layout for a width', () => {
        const draft = draftFromSpec(V2, META, CATALOG)
        const twice = {
            width: 'mobile' as const,
            tracks: 3,
            cells: [
                { col: 'team', line: 1 },
                { col: 'gone', line: 1 },
                { col: 'gone', line: 2 },
                { col: 'team', line: 2 },
            ],
        }
        const again = { width: 'mobile' as const, tracks: 1, cells: [{ col: 'team', line: 1 }] }
        const issues = draftIssues({ ...draft, display: { layouts: [twice, again] } }, CATALOG)
        expect(issues).toEqual([
            {
                path: ['display', 'layouts', 0, 'cells', 1, 'col'],
                message: 'the report has no column "gone"',
            },
            {
                path: ['display', 'layouts', 0, 'cells', 2, 'col'],
                message: 'the report has no column "gone"',
            },
            {
                path: ['display', 'layouts', 0, 'cells', 3, 'col'],
                message: 'a column is placed once per layout',
            },
            { path: ['display', 'layouts', 1, 'width'], message: 'one layout per width' },
        ])
    })

    it('refuses an arrangement with more cells than a report has columns', () => {
        const draft = draftFromSpec(V2, META, CATALOG)
        const cells = Array.from({ length: MAX_COLUMNS + 1 }, () => ({ col: 'team', line: 1 }))
        const many = { width: 'tablet' as const, tracks: 1, cells }
        expect(draftIssues({ ...draft, display: { layouts: [many] } }, CATALOG)).toEqual([
            {
                path: ['display', 'layouts', 0, 'cells'],
                message: `a layout places at most ${MAX_COLUMNS} columns`,
            },
        ])
    })
})

describe('a no-op save keeps what the spec says', () => {
    it('keeps a param that only another param’s options read, through a chain', () => {
        const spec: ReportSpec = {
            version: 2,
            source: SALES_KEY,
            params: [
                {
                    id: 'teams',
                    type: 'choice',
                    label: 'Teams',
                    optionsFrom: {
                        source: TEAMS_KEY,
                        value: [{ field: 'id' }],
                        label: [{ field: 'name' }],
                        where: {
                            op: 'eq',
                            args: [{ path: [{ field: 'region' }] }, { param: 'region' }],
                        },
                    },
                    multiple: true,
                },
                {
                    id: 'region',
                    type: 'choice',
                    label: 'Region',
                    optionsFrom: {
                        source: TEAMS_KEY,
                        value: [{ field: 'region' }],
                        label: [{ field: 'region' }],
                        where: {
                            op: 'startsWith',
                            args: [{ path: [{ field: 'name' }] }, { param: 'stem' }],
                        },
                    },
                },
                { id: 'stem', type: 'text', label: 'Stem' },
                { id: 'unread', type: 'text', label: 'Unread' },
            ],
            columns: [{ id: 'team', label: 'Team', path: [{ link: 'team' }] }],
            filter: { op: 'in', args: [{ path: [{ link: 'team' }] }, { param: 'teams' }] },
        }
        const draft = draftFromSpec(spec, META, CATALOG)
        expect(specFromDraft(draft, CATALOG)).toEqual({
            ...spec,
            params: spec.params?.filter(p => p.id !== 'unread'),
        })
        const kept = { ...spec, params: spec.params?.filter(p => p.id !== 'unread') }
        const again = draftFromSpec(kept, META, CATALOG)
        expect(JSON.stringify(specFromDraft(again, CATALOG))).toBe(JSON.stringify(kept))
    })

    it('keeps defaults the spec spells out', () => {
        const spec: ReportSpec = {
            version: 2,
            source: SALES_KEY,
            columns: [
                { id: 'status', label: 'Status', path: [{ field: 'status' }] },
                {
                    id: 'amount',
                    label: 'Amount',
                    path: [{ field: 'amount' }],
                    aggregate: 'sum',
                    total: 'recompute',
                },
            ],
            sort: [{ col: 'amount', dir: 'asc' }],
            groups: [{ id: 'byStatus', col: 'status' }],
            pivot: { path: [{ field: 'day' }], bucket: 'month', cell: 'amount' },
            detail: {},
            display: {},
        }
        const draft = draftFromSpec(spec, META, CATALOG)
        expect(JSON.stringify(specFromDraft(draft, CATALOG))).toBe(JSON.stringify(spec))
        // A pivot the builder turns on says no maxColumns until one other than the default is chosen.
        const fresh = draftFromSpec({ ...spec, pivot: undefined }, META, CATALOG)
        const on = {
            ...fresh,
            pivot: { ...fresh.pivot, on: true, path: [{ field: 'day' }], cell: 'amount' },
        }
        expect(specFromDraft(on, CATALOG).pivot).toEqual({
            path: [{ field: 'day' }],
            cell: 'amount',
        })
        const twelve = { ...on, pivot: { ...on.pivot, maxColumns: 12 } }
        expect(specFromDraft(twelve, CATALOG).pivot?.maxColumns).toBe(12)
    })
})

// D1: a grouped param from a source can name its group pick, "{group}" standing for
// the group's label.
describe('the group pick text of a param from a source', () => {
    const teamsParam = (groupLabel: string | undefined, grouped = true): ParamDef => ({
        id: 'teams',
        type: 'choice',
        label: 'Teams',
        optionsFrom: {
            source: TEAMS_KEY,
            value: [{ field: 'id' }],
            label: [{ field: 'name' }],
            ...(grouped ? { group: [{ field: 'region' }] } : {}),
            ...(groupLabel === undefined ? {} : { groupLabel }),
            where: { value: true },
        },
        multiple: true,
    })
    const specWith = (param: ParamDef): ReportSpec => ({
        version: 2,
        source: SALES_KEY,
        params: [param],
        columns: [{ id: 'team', label: 'Team', path: [{ link: 'team' }] }],
        filter: { op: 'in', args: [{ path: [{ link: 'team' }] }, { param: 'teams' }] },
    })
    const issues = (param: ParamDef) => {
        const draft = draftFromSpec(specWith(param), META, CATALOG)
        const parsed = builderSchema(CATALOG).safeParse(draft)
        return parsed.success
            ? []
            : parsed.error.issues.map(issue => `${issue.path.join('.')}: ${issue.message}`)
    }

    it('accepts a text that names the group, or none', () => {
        expect(issues(teamsParam('All {group} teams'))).toEqual([])
        expect(issues(teamsParam(''))).toEqual([])
        expect(issues(teamsParam(undefined))).toEqual([])
    })

    it('refuses what the engine refuses (params.go checkGroupLabel)', () => {
        expect(issues(teamsParam('Every team'))).toEqual([
            'params.0.optionsFrom.groupLabel: Write {group} where the group’s name goes',
        ])
        expect(issues(teamsParam(`{group}${'é'.repeat(97)}`))).toEqual([
            'params.0.optionsFrom.groupLabel: Use at most 200 characters',
        ])
        expect(issues(teamsParam('All {group}', false))).toEqual([
            'params.0.optionsFrom.groupLabel: Group the list to name its groups',
        ])
    })

    it('saves a set text in the spec’s key order and leaves out an empty one', () => {
        const set = specWith(teamsParam('All {group} teams'))
        expect(JSON.stringify(specFromDraft(draftFromSpec(set, META, CATALOG), CATALOG))).toBe(
            JSON.stringify(set)
        )
        const empty = draftFromSpec(specWith(teamsParam('')), META, CATALOG)
        expect(specFromDraft(empty, CATALOG)).toEqual(specWith(teamsParam(undefined)))
    })
})

// E1: a grouped param from a source orders its groups (spec.go OptionsFrom.GroupOrder):
// by name, by the group field's declared options, or by a field of the source.
describe('the group order of a param from a source', () => {
    type Order = NonNullable<NonNullable<ParamDef['optionsFrom']>['groupOrder']>
    const teamsParam = (groupOrder: Order, group = [{ field: 'region' }]): ParamDef => ({
        id: 'teams',
        type: 'choice',
        label: 'Teams',
        optionsFrom: {
            source: TEAMS_KEY,
            value: [{ field: 'id' }],
            label: [{ field: 'name' }],
            ...(group.length > 0 ? { group } : {}),
            groupOrder,
            groupLabel: 'All {group} teams',
            where: { value: true },
        },
        multiple: true,
    })
    const specWith = (param: ParamDef): ReportSpec => ({
        version: 2,
        source: SALES_KEY,
        params: [param],
        columns: [{ id: 'team', label: 'Team', path: [{ link: 'team' }] }],
        filter: { op: 'in', args: [{ path: [{ link: 'team' }] }, { param: 'teams' }] },
    })
    const issuePaths = (param: ParamDef) => {
        const draft = draftFromSpec(specWith(param), META, CATALOG)
        const parsed = builderSchema(CATALOG).safeParse(draft)
        return parsed.success ? [] : parsed.error.issues.map(issue => issue.path.join('.'))
    }
    const orders: Order[] = [
        { by: 'label' },
        { by: 'declared' },
        { by: 'path', path: [{ field: 'name' }] },
    ]

    it('saves each order as the spec holds it, through the draft and back', () => {
        for (const order of orders) {
            const spec = specWith(teamsParam(order))
            expect(issuePaths(teamsParam(order))).toEqual([])
            expect(JSON.stringify(specFromDraft(draftFromSpec(spec, META, CATALOG), CATALOG))).toBe(
                JSON.stringify(spec)
            )
        }
    })

    // What the engine refuses (bind.go / params.go): the field's own order of a group
    // that is not a select, and an order with no groups to order.
    it('refuses what the engine refuses', () => {
        expect(issuePaths(teamsParam({ by: 'declared' }, [{ field: 'name' }]))).toEqual([
            'params.0.optionsFrom.groupOrder',
        ])
        expect(issuePaths(teamsParam({ by: 'label' }, []))).toContain(
            'params.0.optionsFrom.groupOrder'
        )
    })
})

// E2: a column that only names its group (the team's region on each of the team's
// rows) is left blank on opened rows (spec.go Column.Detail, "blank").
describe('a column blank on opened rows', () => {
    const BLANK_REGION: ReportSpec = {
        version: CURRENT_SPEC_VERSION,
        source: SALES_KEY,
        columns: [
            { id: 'team', label: 'Team', path: [{ link: 'team' }] },
            {
                id: 'region',
                label: 'Region',
                formula: { record: 'byTeam', path: [{ field: 'region' }] },
                detail: 'blank',
            },
            { id: 'amount', label: 'Amount', path: [{ field: 'amount' }], aggregate: 'sum' },
        ],
        groups: [{ id: 'byTeam', col: 'team' }],
    }
    const issuePaths = (draft: BuilderDraft) => {
        const parsed = builderSchema(CATALOG).safeParse(draft)
        return parsed.success ? [] : parsed.error.issues.map(issue => issue.path.join('.'))
    }
    const labelled = (node: SpecNode): FormulaDraft => ({ kind: 'editable', node })
    const offered = (draft: BuilderDraft) =>
        draft.columns.map((_, index) => offersDetailBlank(draft, index))

    it('keeps a blank column through the draft and back, and a shown one says nothing', () => {
        const draft = draftFromSpec(BLANK_REGION, META, CATALOG)
        expect(draft.columns.map(column => column.detail)).toEqual(['show', 'blank', 'show'])
        expect(issuePaths(draft)).toEqual([])
        const saved = specFromDraft(draft, CATALOG)
        expect(saved).toEqual(BLANK_REGION)
        expect(saved.columns[0]).not.toHaveProperty('detail')
        expect(saved.columns[2]).not.toHaveProperty('detail')
        const shown = {
            ...draft,
            columns: draft.columns.map(column => ({ ...column, detail: 'show' as const })),
        }
        for (const column of specFromDraft(shown, CATALOG).columns) {
            expect(column).not.toHaveProperty('detail')
        }
    })

    it('starts a new report’s column shown on opened rows', () => {
        const source = CATALOG.get(SALES_KEY)
        if (!source) throw new Error('fixture')
        expect(emptyDraft(source).columns.map(column => column.detail)).toEqual(['show'])
    })

    // Offered where the engine accepts it: a grouped report, on any column but the one
    // that names opened rows (the first column with no row label, or the column a row
    // label of exactly {col} is).
    it('offers Leave blank on a grouped report, never on the column that names opened rows', () => {
        const draft = draftFromSpec(BLANK_REGION, META, CATALOG)
        expect(offered(draft)).toEqual([false, true, true])
        expect(offered({ ...draft, groups: [] })).toEqual([false, false, false])
        expect(offered({ ...draft, detailLabel: labelled({ col: 'region' }) })).toEqual([
            true,
            false,
            true,
        ])
        const overTeam = labelled({ op: 'concat', args: [{ col: 'team' }, { value: '!' }] })
        expect(offered({ ...draft, detailLabel: overTeam })).toEqual([true, true, true])
        const byCustomer = labelled({ path: [{ field: 'customer' }] })
        expect(offered({ ...draft, detailLabel: byCustomer })).toEqual([true, true, true])
    })

    it('refuses a blank column where the engine refuses it (bind.go columns.N.detail)', () => {
        const draft = draftFromSpec(BLANK_REGION, META, CATALOG)
        expect(issuePaths({ ...draft, groups: [] })).toContain('columns.1.detail')
        const first = draft.columns.map((column, i) =>
            i === 0 ? { ...column, detail: 'blank' as const } : column
        )
        expect(issuePaths({ ...draft, columns: first })).toContain('columns.0.detail')
        expect(issuePaths({ ...draft, detailLabel: labelled({ col: 'region' }) })).toContain(
            'columns.1.detail'
        )
    })
})
