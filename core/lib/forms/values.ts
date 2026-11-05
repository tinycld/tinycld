import { type ZodTypeAny, z } from 'zod'
import { applyComputed } from './compute'
import {
    type CustomField,
    dateValueSchema,
    type FormDefinition,
    type FormField,
    type HostTableColumn,
    type TableColumn,
} from './definition'
import { type FormHostBuiltin, type FormHostDef, findBuiltin } from './host'
import type { FormState, FormValues } from './logic'
import { completeTableRows } from './table-rows'

export interface ChecklistItem {
    id: string
    text: string
    done: boolean
}
export type TableRow = { id: string } & Record<string, unknown>
export const checklistItemSchema = z.object({
    id: z.string().min(1),
    text: z.string(),
    done: z.boolean(),
})

/** Resolves what a field really is: a custom field as declared, or the host builtin behind a builtin ref. */
export function resolveKind(
    host: FormHostDef,
    field: FormField
): { kind: string; builtin?: FormHostBuiltin; custom?: CustomField } {
    if (field.kind === 'builtin') {
        const builtin = findBuiltin(host, field.key)
        return { kind: builtin?.kind ?? 'custom', builtin }
    }
    return { kind: field.kind, custom: field }
}

/** True when the field's value travels in the submit payload (not rows, sections or collaborative text). */
export function isSubmittedKind(host: FormHostDef, field: FormField): boolean {
    const { kind, builtin, custom } = resolveKind(host, field)
    if (kind === 'section') return false
    if (builtin?.storage === 'rows') return false
    if (builtin?.collaborative) return false
    if (custom && 'collaborative' in custom && custom.collaborative) return false
    return true
}

interface KindSchemaInput {
    custom?: CustomField
    builtin?: FormHostBuiltin
    required: boolean
    multiple: boolean
    missing: string
    label: string
    options?: { value: string }[]
}

/**
 * A required field with no value at all (never typed, so not even `''`) must
 * read as missing, not as zod's "expected string, received undefined".
 */
function absent(missing: string) {
    return {
        error: (issue: { input?: unknown }) =>
            issue.input === undefined || issue.input === null ? missing : undefined,
    }
}

function textLimit({ custom, builtin }: KindSchemaInput): number | undefined {
    if (custom?.kind === 'text' || custom?.kind === 'textarea') return custom.maxLength
    return builtin?.kind === 'text' ? builtin.maxLength : undefined
}

function textSchema(input: KindSchemaInput): ZodTypeAny {
    const { required, missing, label } = input
    let t = z.string(absent(missing))
    const limit = textLimit(input)
    // Same words as the server's refusal (checkKind), so a refusal reads the same either way.
    if (limit) t = t.max(limit, `${label} is too long`)
    return required ? t.trim().min(1, missing) : t.nullable()
}

function numberSchema({ custom, required, missing }: KindSchemaInput): ZodTypeAny {
    let n = z.number(absent(missing))
    if (custom?.kind === 'number') {
        if (custom.min !== undefined) n = n.min(custom.min)
        if (custom.max !== undefined) n = n.max(custom.max)
    }
    return required ? n : n.nullable()
}

function dateSchema({ custom, required, missing }: KindSchemaInput): ZodTypeAny {
    // A built-in date is a record column in the database's own format; a custom date is
    // stored as typed, so it has the shape the server checks (DATE_RE).
    const d = custom ? dateValueSchema : z.string()
    return required ? z.string(absent(missing)).min(1, missing).pipe(d) : d.nullable()
}

function selectSchema(input: KindSchemaInput): ZodTypeAny {
    const { custom, required, multiple, missing, label, options } = input
    const allowed = (options ?? (custom?.kind === 'select' ? custom.options : [])).map(o => o.value)
    const one =
        allowed.length > 0
            ? z.enum(allowed as [string, ...string[]], absent(missing))
            : z.never({ message: `${label} has no options` })
    if (!multiple) return required ? one : one.nullable()
    const many = z.array(one, absent(missing))
    return required ? many.min(1, missing) : many.nullable()
}

function idSchema({ required, multiple, missing }: KindSchemaInput): ZodTypeAny {
    if (!multiple)
        return required ? z.string(absent(missing)).min(1, missing) : z.string().nullable()
    const many = z.array(z.string(), absent(missing))
    return required ? many.min(1, missing) : many.nullable()
}

function listSchema({ required, missing }: KindSchemaInput): ZodTypeAny {
    const items = z.array(checklistItemSchema, absent(missing))
    return required ? items.min(1, missing) : items.nullable()
}

function anySchema({ required, missing }: KindSchemaInput): ZodTypeAny {
    if (!required) return z.unknown()
    return z.unknown().refine(v => v !== null && v !== undefined && v !== '', missing)
}

/** The number input holds `''` while cleared: that is no value, not a wrong one. */
const blankToNull = (value: unknown) => (value === '' ? null : value)

function columnSchemaFor(c: TableColumn): ZodTypeAny {
    const needed = c.required && !c.hidden
    const missing = `${c.label} is required`
    const optional = (s: ZodTypeAny) => s.nullable().optional()
    switch (c.kind) {
        case 'text': {
            const text = z.string(absent(missing))
            const t = c.maxLength ? text.max(c.maxLength) : text
            return needed ? t.trim().min(1, missing) : optional(t)
        }
        case 'number': {
            let n = z.number(absent(missing))
            if (c.min !== undefined) n = n.min(c.min)
            if (c.max !== undefined) n = n.max(c.max)
            return z.preprocess(blankToNull, needed ? n : optional(n))
        }
        case 'toggle':
            return optional(z.boolean())
        case 'date':
            return needed
                ? z.string(absent(missing)).min(1, missing).pipe(dateValueSchema)
                : optional(dateValueSchema)
        case 'select': {
            const values = c.options.map(o => o.value) as [string, ...string[]]
            const one = z.enum(values, absent(missing))
            return needed ? one : optional(one)
        }
        case 'computed':
            // The engine overwrites it; whatever the client held is not validated.
            return z.unknown().optional()
    }
}

function tableSchema({ custom, builtin, required, missing }: KindSchemaInput): ZodTypeAny {
    const columns = custom?.kind === 'table' ? custom.columns : (builtin?.columns ?? [])
    const shape: Record<string, ZodTypeAny> = { id: z.string().min(1) }
    for (const c of columns) shape[c.key] = columnSchemaFor(c)
    const rows = z.array(z.object(shape).passthrough(), absent(missing))
    return required ? rows.min(1, missing) : rows.nullable()
}

const SCHEMA_BY_KIND: Record<string, (input: KindSchemaInput) => ZodTypeAny> = {
    text: textSchema,
    textarea: textSchema,
    richtext: textSchema,
    markdown: textSchema,
    number: numberSchema,
    toggle: ({ required, missing }) =>
        required ? z.literal(true, { message: missing }) : z.boolean().nullable(),
    date: dateSchema,
    select: selectSchema,
    user: idSchema,
    file: idSchema,
    checklist: listSchema,
    table: tableSchema,
}

function schemaFor(
    host: FormHostDef,
    field: FormField,
    required: boolean,
    options?: { value: string }[]
): ZodTypeAny {
    const { kind, custom, builtin } = resolveKind(host, field)
    const multiple =
        (custom && 'multiple' in custom && custom.multiple) || builtin?.multiple || false
    const build = SCHEMA_BY_KIND[kind] ?? anySchema
    const s = build({
        custom,
        builtin,
        required,
        multiple,
        missing: `${field.label} is required`,
        label: field.label,
        options,
    })
    return required ? s : s.optional()
}

/** A host component's value schema, keyed by the built-in's component name. */
export type HostSchemas = Readonly<Record<string, ZodTypeAny>>

/**
 * Core still owns presence: a missing required value reads as every other kind's missing
 * text (`''` counts as missing), and an optional one passes null or undefined. Any other value,
 * `''` on an optional field included, goes to the host schema.
 */
function hostValueSchema(own: ZodTypeAny, required: boolean, missing: string): ZodTypeAny {
    if (!required) return own.nullable().optional()
    return z
        .unknown()
        .refine(v => v !== null && v !== undefined && v !== '', missing)
        .pipe(own)
}

/**
 * The submit schema for the fields visible in this state. Rows, sections and collaborative
 * text are excluded. A custom built-in whose component is a key of `schemas` takes that schema.
 */
export function buildSchema(
    def: FormDefinition,
    host: FormHostDef,
    state: FormState,
    schemas: HostSchemas = {}
) {
    const shape: Record<string, ZodTypeAny> = {}
    for (const f of def.fields) {
        // A host table's rows are not submitted with the record, but they are validated here.
        if (!isSubmittedKind(host, f) && !isHostTable(host, f)) continue
        const { kind, builtin } = resolveKind(host, f)
        if (kind === 'computed') continue
        const st = state.fields[f.key]
        if (!st?.visible) continue
        // A host component knows its value's shape and its own messages; core cannot
        // read them from the definition.
        const own = kind === 'custom' && builtin?.component ? schemas[builtin.component] : undefined
        if (own) shape[f.key] = hostValueSchema(own, st.required, `${f.label} is required`)
        else shape[f.key] = schemaFor(host, f, st.required, st.options)
    }
    // Passthrough keeps hidden, row and collaborative values in the output, so the resolver can
    // hand them to splitValues. Only the keys in `shape` are validated.
    return z.object(shape).passthrough()
}

export interface SplitValues {
    columns: Record<string, unknown>
    fields: Record<string, unknown>
    rows: Record<string, ChecklistItem[]>
    /** Rows of a host `table` built-in, which live in the host's own collection. */
    tables: Record<string, TableRow[]>
}

/** Routes each value to the record column, the custom-field JSON column, or its own rows table. */
export function splitValues(
    host: FormHostDef,
    def: FormDefinition,
    values: FormValues
): SplitValues {
    const out: SplitValues = { columns: {}, fields: {}, rows: {}, tables: {} }
    for (const f of def.fields) {
        const v = values[f.key]
        if (v === undefined) continue
        const { kind, builtin } = resolveKind(host, f)
        if (kind === 'section') continue
        if (builtin?.storage === 'rows') {
            if (kind === 'table') out.tables[f.key] = Array.isArray(v) ? (v as TableRow[]) : []
            else out.rows[f.key] = Array.isArray(v) ? (v as ChecklistItem[]) : []
        } else if (builtin) out.columns[f.key] = v
        else out.fields[f.key] = v
    }
    return out
}

function storedValueOf(
    host: FormHostDef,
    field: FormField,
    record: Record<string, unknown>,
    rows: Record<string, ChecklistItem[]>,
    tables: Record<string, TableRow[]>
): unknown {
    const { kind, builtin } = resolveKind(host, field)
    // A host table the caller did not load stays undefined, so totals over it keep their stored value.
    if (builtin?.storage === 'rows')
        return kind === 'table' ? tables[field.key] : (rows[field.key] ?? [])
    if (builtin) return record[field.key]
    const json = (record[host.fieldsColumn] ?? {}) as Record<string, unknown>
    return json[field.key]
}

/** The inverse of splitValues. A null JSON column reads as no custom values. */
export function mergeValues(
    host: FormHostDef,
    def: FormDefinition,
    record: Record<string, unknown>,
    rows: Record<string, ChecklistItem[]> = {},
    tables: Record<string, TableRow[]> = {}
): FormValues {
    const out: FormValues = {}
    for (const f of def.fields) {
        if (resolveKind(host, f).kind === 'section') continue
        const value = storedValueOf(host, f, record, rows, tables)
        const columns = tableColumnsOf(host, f)
        out[f.key] =
            columns && Array.isArray(value)
                ? completeTableRows(columns, value as TableRow[])
                : value
    }
    return out
}

/** A host table built-in: its rows live in the host's own collection. */
export function isHostTable(host: FormHostDef, field: FormField): boolean {
    const { kind, builtin } = resolveKind(host, field)
    return kind === 'table' && builtin?.storage === 'rows'
}

/** A table field's columns, custom or host; undefined for any other field. */
export function tableColumnsOf(host: FormHostDef, field: FormField): HostTableColumn[] | undefined {
    const { kind, custom, builtin } = resolveKind(host, field)
    if (kind !== 'table') return undefined
    if (custom?.kind === 'table') return custom.columns
    return builtin?.columns
}

/** No rows in every table, so a new record's totals read as 0, not as rows not loaded. */
export function emptyTablesOf(def: FormDefinition, host: FormHostDef): FormValues {
    const out: FormValues = {}
    for (const f of def.fields) if (tableColumnsOf(host, f)) out[f.key] = []
    return out
}

export function computedKeysOf(def: FormDefinition, host: FormHostDef): string[] {
    return def.fields.filter(f => resolveKind(host, f).kind === 'computed').map(f => f.key)
}

/** What a write sends: every table's rows completed (a cleared number is null), then the computed values. */
export function prepareValues(
    def: FormDefinition,
    host: FormHostDef,
    values: FormValues
): FormValues {
    const out: FormValues = { ...values }
    for (const f of def.fields) {
        const columns = tableColumnsOf(host, f)
        const rows = out[f.key]
        if (columns && Array.isArray(rows))
            out[f.key] = completeTableRows(columns, rows as TableRow[])
    }
    return applyComputed(def, host, out)
}
