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
