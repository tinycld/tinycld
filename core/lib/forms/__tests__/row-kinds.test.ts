import { describe, expect, it } from 'vitest'
import { z } from 'zod'
import { defaultDefinitionFor, type FormHostDefInput, hostDefSchema } from '../host'
import { evaluate, type FormValues } from '../logic'
import { duplicateRow, insertRow, newTableRow, replaceRows } from '../table-rows'
import { buildSchema } from '../values'

const ctx = { today: '2026-01-01', userId: 'u' }

const columns = [
    { key: 'product', label: 'Product', kind: 'text', required: true },
    { key: 'qty', label: 'Qty', kind: 'number' },
    { key: 'text', label: 'Text', kind: 'text', required: true },
]

const itemKind = { key: 'item', label: 'Item', addLabel: 'Add line', columns: ['product', 'qty'] }
const noteKind = {
    key: 'note',
    label: 'Note',
    addLabel: 'Note line',
    columns: ['text'],
    span: true,
}

/** A host whose `lines` rows are items or notes; `rowKinds` overrides the kinds. */
function hostInput(rowKinds: unknown[] = [itemKind, noteKind]): FormHostDefInput {
    return {
        key: 'gizmos:bill',
        label: 'Bill',
        collection: 'gizmos_bills',
        formColumn: 'form',
        fieldsColumn: 'fields',
        columns: ['name', 'form', 'fields'],
        builtin: [
            { key: 'name', label: 'Name', kind: 'text', storage: 'column', required: true },
            {
                key: 'lines',
                label: 'Lines',
                kind: 'table',
                storage: 'rows',
                columns,
                rowKinds,
            },
        ],
    } as FormHostDefInput
}

const issuesOf = (input: unknown) => {
    const result = hostDefSchema.safeParse(input)
    return result.success ? [] : result.error.issues.map(i => i.path.join('.'))
}

describe('host row kinds', () => {
    it('keeps the row kinds of a table built-in', () => {
        const host = hostDefSchema.parse(hostInput())
        expect(host.builtin[1].rowKinds).toMatchObject([
            { ...itemKind, span: false },
            { ...noteKind, span: true },
        ])
    })

    it('refuses a kind that names a column the table lacks', () => {
        const bad = hostInput([{ ...itemKind, columns: ['product', 'ghost'] }])
        expect(issuesOf(bad).some(p => p.startsWith('builtin.1.rowKinds.0.columns'))).toBe(true)
    })

    it('refuses row kinds on a built-in that is not a table', () => {
        const input = hostInput()
        const bad = {
            ...input,
            builtin: [{ ...input.builtin[0], rowKinds: [itemKind] }, input.builtin[1]],
        }
        expect(hostDefSchema.safeParse(bad).success).toBe(false)
        expect(issuesOf(bad).some(p => p.startsWith('builtin.0.rowKinds'))).toBe(true)
    })

    it('refuses two kinds with one key', () => {
        const bad = hostInput([itemKind, { ...noteKind, key: 'item' }])
        expect(hostDefSchema.safeParse(bad).success).toBe(false)
    })

    it('accepts rowShortcuts on a table without kinds', () => {
        const input = hostInput()
        const { rowKinds: _, ...lines } = input.builtin[1] as Record<string, unknown>
        const ok = { ...input, builtin: [input.builtin[0], { ...lines, rowShortcuts: true }] }
        const parsed = hostDefSchema.safeParse(ok)
        expect(parsed.success).toBe(true)
        expect(parsed.data?.builtin[1].rowShortcuts).toBe(true)
    })

    it('refuses a columnAt that maps a column the kind lacks, or to a slot the first kind lacks', () => {
        const pct = (columnAt: Record<string, string>) => ({
            key: 'pct',
            label: 'Percent',
            addLabel: 'Percent line',
            columns: ['product', 'text'],
            columnAt,
        })
        expect(hostDefSchema.safeParse(hostInput([itemKind, pct({ text: 'qty' })])).success).toBe(
            true
        )
        expect(hostDefSchema.safeParse(hostInput([itemKind, pct({ qty: 'qty' })])).success).toBe(
            false
        )
        expect(hostDefSchema.safeParse(hostInput([itemKind, pct({ text: 'text' })])).success).toBe(
            false
        )
    })
})

describe('row kinds in the submit schema', () => {
    const host = hostDefSchema.parse(hostInput())
    const def = defaultDefinitionFor(host)
    const parse = (values: FormValues, recordSchema?: z.ZodTypeAny) =>
        buildSchema(def, host, evaluate(def, values, ctx), {}, recordSchema).safeParse(values)
    const problems = (values: FormValues, recordSchema?: z.ZodTypeAny) => {
        const result = parse(values, recordSchema)
        return result.success
            ? []
            : result.error.issues.map(i => ({ path: i.path.join('.'), message: i.message }))
    }

    it("checks an item row against the item's columns only", () => {
        expect(problems({ name: 'N', lines: [{ id: 'r1', kind: 'item', product: null }] })).toEqual(
            [{ path: 'lines.0.product', message: 'Product is required' }]
        )
    })

    it('does not check product on a note row', () => {
        const values = { name: 'N', lines: [{ id: 'r1', kind: 'note', text: 'x', product: null }] }
        expect(parse(values).success).toBe(true)
    })

    it('refuses a row of a kind the table does not have', () => {
        const paths = problems({ name: 'N', lines: [{ id: 'r1', kind: 'other' }] }).map(p => p.path)
        expect(paths).toContain('lines.0.kind')
    })
})

const lineColumns = [
    { key: 'product', label: 'Product', kind: 'text', required: true },
    { key: 'description', label: 'Description', kind: 'text' },
    { key: 'qty', label: 'Qty', kind: 'number' },
    { key: 'rate', label: 'Rate', kind: 'number' },
    { key: 'note', label: 'Note', kind: 'text', hidden: true },
    { key: 'taxable', label: 'Tax', kind: 'toggle' },
]

const lineItem = {
    key: 'item',
    label: 'Item',
    addLabel: 'Add line',
    columns: ['product', 'description', 'qty', 'taxable'],
}
const lineDescription = {
    key: 'description',
    label: 'Description',
    addLabel: 'Description line',
    columns: ['description'],
    span: true,
    required: ['description'],
}
const linePercent = {
    key: 'percent',
    label: 'Percent',
    addLabel: 'Percent line',
    columns: ['product', 'rate', 'note', 'taxable'],
    required: ['rate', 'note', 'taxable'],
}

/** A `lines` table whose kinds name the columns each must fill. */
function requiredHostInput(
    rowKinds: unknown[] = [lineItem, lineDescription, linePercent]
): FormHostDefInput {
    const input = hostInput(rowKinds)
    return {
        ...input,
        builtin: [input.builtin[0], { ...input.builtin[1], columns: lineColumns }],
    } as FormHostDefInput
}

const issueList = (input: unknown) => {
    const result = hostDefSchema.safeParse(input)
    return result.success
        ? []
        : result.error.issues.map(i => ({ path: i.path.join('.'), message: i.message }))
}

describe('host row kinds: required columns per kind', () => {
    it('keeps a kind whose required columns are its own columns', () => {
        const parsed = hostDefSchema.safeParse(requiredHostInput())
        expect(parsed.success).toBe(true)
        expect(parsed.data?.builtin[1].rowKinds?.[1].required).toEqual(['description'])
        expect(parsed.data?.builtin[1].rowKinds?.[2].required).toEqual(['rate', 'note', 'taxable'])
    })

    it('refuses a required key that is not a column of the table', () => {
        const bad = requiredHostInput([
            lineItem,
            { ...lineDescription, required: ['description', 'ghost'] },
        ])
        expect(issueList(bad)).toContainEqual({
            path: 'builtin.1.rowKinds.1.required.1',
            message: 'ghost is not a column of this kind',
        })
    })

    it("refuses a required key that is a table column but not one of the kind's columns", () => {
        const bad = requiredHostInput([lineItem, { ...lineDescription, required: ['product'] }])
        expect(issueList(bad)).toContainEqual({
            path: 'builtin.1.rowKinds.1.required.0',
            message: 'product is not a column of this kind',
        })
    })

    it('refuses a required key named twice', () => {
        const bad = requiredHostInput([
            lineItem,
            { ...lineDescription, required: ['description', 'description'] },
        ])
        expect(hostDefSchema.safeParse(bad).success).toBe(false)
        expect(issueList(bad)).toContainEqual({
            path: 'builtin.1.rowKinds.1.required.1',
            message: 'duplicate required column description',
        })
    })

    it('parses a kind with no required list as before, beside a kind with one', () => {
        const parsed = hostDefSchema.safeParse(requiredHostInput([lineItem, lineDescription]))
        expect(parsed.success).toBe(true)
        expect(parsed.data?.builtin[1].rowKinds?.[0].required).toBeUndefined()
        expect(parsed.data?.builtin[1].rowKinds?.[0]).toMatchObject({ ...lineItem, span: false })
        expect(parsed.data?.builtin[1].rowKinds?.[1].required).toEqual(['description'])
    })
})

describe('row kinds: required columns per kind in the submit schema', () => {
    const host = hostDefSchema.parse(requiredHostInput())
    const def = defaultDefinitionFor(host)
    const problems = (lines: Record<string, unknown>[]) => {
        const values = { name: 'N', lines }
        const result = buildSchema(def, host, evaluate(def, values, ctx), {}).safeParse(values)
        return result.success
            ? []
            : result.error.issues.map(i => ({ path: i.path.join('.'), message: i.message }))
    }
    const item = (cells: Record<string, unknown> = {}) => ({
        id: 'i1',
        kind: 'item',
        product: 'Widget',
        description: null,
        qty: 1,
        taxable: false,
        ...cells,
    })
    const descriptionRow = (cells: Record<string, unknown>) => ({
        id: 'd1',
        kind: 'description',
        ...cells,
    })
    const percent = (cells: Record<string, unknown> = {}) => ({
        id: 'p1',
        kind: 'percent',
        product: 'Discount',
        rate: 10,
        note: null,
        taxable: false,
        ...cells,
    })
    const descriptionMissing = [{ path: 'lines.1.description', message: 'Description is required' }]

    it('refuses a description row with an empty description, and accepts a filled one', () => {
        expect(problems([item(), descriptionRow({ description: '' })])).toEqual(descriptionMissing)
        expect(problems([item(), descriptionRow({ description: 'Labor' })])).toEqual([])
    })

    it('refuses a description row whose description is only spaces', () => {
        expect(problems([item(), descriptionRow({ description: '   ' })])).toEqual(
            descriptionMissing
        )
    })

    it('refuses a description row with no description cell', () => {
        expect(problems([item(), descriptionRow({})])).toEqual(descriptionMissing)
        expect(problems([item(), descriptionRow({ description: null })])).toEqual(
            descriptionMissing
        )
    })

    it('lets an item row leave the description empty while a description row may not', () => {
        for (const empty of ['', '   ', null])
            expect(
                problems([item({ description: empty }), descriptionRow({ description: '' })])
            ).toEqual(descriptionMissing)
    })

    it('still requires a table-wide required column on every kind that draws it', () => {
        expect(
            problems([
                item({ product: '' }),
                descriptionRow({ description: '' }),
                percent({ product: null }),
            ])
        ).toEqual([
            { path: 'lines.0.product', message: 'Product is required' },
            { path: 'lines.1.description', message: 'Description is required' },
            { path: 'lines.2.product', message: 'Product is required' },
        ])
    })

    it("requires a number column in a kind's list, and the cleared input is no value", () => {
        expect(problems([percent({ rate: '' })])).toEqual([
            { path: 'lines.0.rate', message: 'Rate is required' },
        ])
        expect(problems([percent({ rate: null })])).toEqual([
            { path: 'lines.0.rate', message: 'Rate is required' },
        ])
    })

    const rateMissing = [{ path: 'lines.0.rate', message: 'Rate is required' }]

    it("never requires a hidden column, even in a kind's list", () => {
        expect(problems([percent({ rate: null, note: null })])).toEqual(rateMissing)
        expect(problems([percent({ rate: null, note: '' })])).toEqual(rateMissing)
    })

    it("never fails a toggle column in a kind's list when it is off", () => {
        expect(problems([percent({ rate: null, taxable: false })])).toEqual(rateMissing)
        expect(problems([percent({ rate: null, taxable: null })])).toEqual(rateMissing)
    })
})

describe('row kinds: a needed select or date cell that is blank', () => {
    const visitColumns = [
        { key: 'product', label: 'Product', kind: 'text' },
        {
            key: 'unit',
            label: 'Unit',
            kind: 'select',
            options: [
                { value: 'hour', label: 'Hour' },
                { value: 'day', label: 'Day' },
            ],
        },
        { key: 'visit', label: 'Visit date', kind: 'date' },
        {
            key: 'zone',
            label: 'Zone',
            kind: 'select',
            required: true,
            options: [{ value: 'north', label: 'North' }],
        },
        { key: 'start', label: 'Start date', kind: 'date', required: true },
    ]
    const serviceKind = {
        key: 'service',
        label: 'Service',
        addLabel: 'Add service',
        columns: ['product', 'unit', 'visit', 'zone', 'start'],
        required: ['unit', 'visit'],
    }
    const input = hostInput([serviceKind])
    const host = hostDefSchema.parse({
        ...input,
        builtin: [input.builtin[0], { ...input.builtin[1], columns: visitColumns }],
    })
    const def = defaultDefinitionFor(host)
    const problems = (row: Record<string, unknown>) => {
        const values = { name: 'N', lines: [{ id: 's1', kind: 'service', ...row }] }
        const result = buildSchema(def, host, evaluate(def, values, ctx), {}).safeParse(values)
        return result.success
            ? []
            : result.error.issues.map(i => ({ path: i.path.join('.'), message: i.message }))
    }
    const filled = {
        product: 'Visit',
        unit: 'hour',
        visit: '2026-01-02',
        zone: 'north',
        start: '2026-01-01',
    }

    it("gives '<Label> is required' for a kind's required select and date cells left blank", () => {
        expect(problems(filled)).toEqual([])
        for (const blank of ['   ', ''])
            expect(problems({ ...filled, unit: blank, visit: blank })).toEqual([
                { path: 'lines.0.unit', message: 'Unit is required' },
                { path: 'lines.0.visit', message: 'Visit date is required' },
            ])
    })

    it("gives '<Label> is required' for table-wide required select and date cells left blank", () => {
        for (const blank of ['   ', ''])
            expect(problems({ ...filled, zone: blank, start: blank })).toEqual([
                { path: 'lines.0.zone', message: 'Zone is required' },
                { path: 'lines.0.start', message: 'Start date is required' },
            ])
    })
})

describe('row helpers', () => {
    const host = hostDefSchema.parse(hostInput())
    const tableColumns = host.builtin[1].columns ?? []
    const a = { id: 'a', kind: 'item', product: 'A', qty: 1, text: null }
    const b = { id: 'b', kind: 'note', product: null, qty: null, text: 'B' }
    const c = { id: 'c', kind: 'item', product: 'C', qty: 3, text: null }

    it('makes a blank row of a kind', () => {
        expect(newTableRow(tableColumns, 'r1', 'note')).toEqual({
            id: 'r1',
            kind: 'note',
            product: null,
            qty: null,
            text: null,
        })
    })

    it('inserts a row at an index', () => {
        const rows = [a, b]
        expect(insertRow(rows, 1, c)).toEqual([a, c, b])
        expect(insertRow(rows, 0, c)).toEqual([c, a, b])
        expect(insertRow(rows, 2, c)).toEqual([a, b, c])
        expect(rows).toEqual([a, b])
    })

    it('duplicates a row right below it, with a new id and the same cells and kind', () => {
        const rows = [a, b]
        expect(duplicateRow(rows, 'a', 'a2')).toEqual([a, { ...a, id: 'a2' }, b])
        expect(rows).toEqual([a, b])
    })

    it('replaces a row by the given rows at its place', () => {
        const x = { id: 'x', kind: 'item', product: 'X', qty: 1, text: null }
        const y = { id: 'y', kind: 'note', product: null, qty: null, text: 'Y' }
        const rows = [a, b, c]
        expect(replaceRows(rows, 'b', [x, y])).toEqual([a, x, y, c])
        expect(rows).toEqual([a, b, c])
    })
})

describe('a whole-record host schema', () => {
    const host = hostDefSchema.parse(hostInput())
    const def = defaultDefinitionFor(host)
    const later = z
        .object({ a: z.string() })
        .superRefine((_, ctx) => ctx.addIssue({ code: 'custom', path: ['due'], message: 'later' }))
    const problems = (values: FormValues) => {
        const result = buildSchema(def, host, evaluate(def, values, ctx), {}, later).safeParse(
            values
        )
        return result.success
            ? []
            : result.error.issues.map(i => ({ path: i.path.join('.'), message: i.message }))
    }

    it('refuses the record with its issue at its path', () => {
        expect(problems({ name: 'N', a: 'x', lines: [] })).toEqual([
            { path: 'due', message: 'later' },
        ])
    })

    it("reports its issues after the field schemas' own", () => {
        expect(problems({ name: '', a: 'x', lines: [] })).toEqual([
            { path: 'name', message: 'Name is required' },
            { path: 'due', message: 'later' },
        ])
    })
})
