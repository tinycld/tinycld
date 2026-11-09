import { z } from 'zod'
import { computedIssues } from './computed-validation'
import {
    CUSTOM_KINDS,
    type DefinitionIssue,
    definitionSchema,
    FIELD_WIDTHS,
    type FormDefinition,
    hostColumnSchema,
    keySchema,
    ruleSchema,
    validateDefinitionForHost,
} from './definition'
import { computeExprSchema } from './expression'

export const BUILTIN_KINDS = [...CUSTOM_KINDS, 'custom', 'section'] as const
export type BuiltinKind = (typeof BUILTIN_KINDS)[number]

/**
 * One kind of row of a host table (an item line, a description line). A row
 * stores its kind in its `kind` cell and draws only its kind's columns.
 */
const rowKindSchema = z.object({
    key: keySchema,
    label: z.string().min(1).max(60),
    /** The add action's label ("Add line", "Description line"). */
    addLabel: z.string().min(1).max(60),
    /** The table columns a row of this kind draws, in order. */
    columns: z.array(keySchema).min(1),
    /** The kind's one cell spans the whole row (a description or subtotal line). */
    span: z.boolean().default(false),
    /**
     * Column of this kind → the first kind's column whose header slot it is
     * drawn in on the wide grid (a percent cell under Rate).
     */
    columnAt: z.record(keySchema, keySchema).optional(),
    /** Key combo that adds a row of this kind after the focused row. */
    shortcut: z.string().min(1).optional(),
})
export type HostRowKind = z.infer<typeof rowKindSchema>

type BuiltinIssueSink = (path: (string | number)[], message: string) => void

/** Row kinds belong to a table, name its columns, and place cells in the first kind's slots. */
function rowKindIssues(
    rowKinds: HostRowKind[],
    columns: { key: string }[] | undefined,
    report: BuiltinIssueSink
) {
    const columnKeys = new Set((columns ?? []).map(c => c.key))
    const firstKind = new Set(rowKinds[0]?.columns ?? [])
    const kindKeys = new Set<string>()
    rowKinds.forEach((kind, k) => {
        if (kindKeys.has(kind.key))
            report(['rowKinds', k, 'key'], `duplicate row kind key ${kind.key}`)
        kindKeys.add(kind.key)
        kind.columns.forEach((column, c) => {
            if (!columnKeys.has(column))
                report(['rowKinds', k, 'columns', c], `${column} is not a column of this table`)
        })
        if (kind.span && kind.columns.length !== 1)
            report(['rowKinds', k, 'columns'], 'a span row kind has exactly one column')
        const own = new Set(kind.columns)
        for (const [column, slot] of Object.entries(kind.columnAt ?? {})) {
            if (!own.has(column))
                report(
                    ['rowKinds', k, 'columnAt', column],
                    `${column} is not a column of this kind`
                )
            if (!firstKind.has(slot))
                report(
                    ['rowKinds', k, 'columnAt', column],
                    `${slot} is not a column of the first row kind`
                )
        }
    })
}

const builtinSchema = z
    .object({
        key: keySchema,
        label: z.string().min(1).max(120),
        kind: z.enum(BUILTIN_KINDS),
        storage: z.enum(['column', 'rows', 'none']),
        required: z.boolean().default(false),
        /**
         * An optional built-in the record depends on: an admin may move and
         * relabel it, never hide it (field flag, rule, or hidden page).
         */
        locked: z.boolean().default(false),
        multiple: z.boolean().default(false),
        collaborative: z.boolean().default(false),
        /** Export name in the host's UI module `components` map. */
        component: z.string().min(1).optional(),
        /**
         * `bare`: on a record (edit and read) the host component draws
         * everything; core passes it no error, required mark or description, so
         * a bare built-in must validate and label itself. The cell is only the
         * width wrapper, and only when the component exists. The design canvas
         * still draws the labelled cell.
         */
        chrome: z.enum(['cell', 'bare']).default('cell'),
        /** A computed built-in: the formula the engine and the server evaluate. */
        expression: computeExprSchema.optional(),
        decimals: z.number().int().min(0).max(6).default(0),
        /** A table built-in: its columns. Rows live in the host's own collection. */
        columns: z.array(hostColumnSchema).min(1).optional(),
        /**
         * A text built-in's most characters. A host's text column has a length
         * the database enforces; the form says so before the save, and the
         * server gives every other writer the same refusal.
         */
        maxLength: z.number().int().positive().max(10000).optional(),
        /** A table built-in's kinds of row; absent means one kind with every column. */
        rowKinds: z.array(rowKindSchema).min(1).optional(),
        /** The table registers core's row keys (remove, move, duplicate, open menu). */
        rowShortcuts: z.boolean().default(false),
    })
    .superRefine((b, ctx) => {
        if (b.rowKinds) {
            if (b.kind !== 'table')
                ctx.addIssue({
                    code: 'custom',
                    path: ['rowKinds'],
                    message: 'only a table built-in has row kinds',
                })
            else
                rowKindIssues(b.rowKinds, b.columns, (path, message) =>
                    ctx.addIssue({ code: 'custom', path, message })
                )
        }
        if (b.maxLength !== undefined && b.kind !== 'text')
            ctx.addIssue({
                code: 'custom',
                path: ['maxLength'],
                message: 'maxLength is for a text built-in',
            })
        if (b.kind === 'section' && b.storage !== 'none')
            ctx.addIssue({ code: 'custom', path: ['storage'], message: 'a section has no storage' })
        if (b.kind === 'section' && !b.component)
            ctx.addIssue({
                code: 'custom',
                path: ['component'],
                message: 'a section needs a component',
            })
        if (b.kind === 'custom' && !b.component)
            ctx.addIssue({
                code: 'custom',
                path: ['component'],
                message: 'a custom kind needs a component',
            })
        if (b.collaborative && b.kind !== 'markdown' && b.kind !== 'richtext')
            ctx.addIssue({
                code: 'custom',
                path: ['collaborative'],
                message: 'only markdown and richtext can be collaborative',
            })
        if (b.kind === 'computed') {
            if (b.expression === undefined)
                ctx.addIssue({
                    code: 'custom',
                    path: ['expression'],
                    message: 'a computed built-in needs an expression',
                })
            if (b.storage !== 'column')
                ctx.addIssue({
                    code: 'custom',
                    path: ['storage'],
                    message: 'a computed built-in is stored in a column',
                })
        } else if (b.expression !== undefined) {
            ctx.addIssue({
                code: 'custom',
                path: ['expression'],
                message: 'only a computed built-in has an expression',
            })
        }
        if (b.kind === 'table') {
            if (!b.columns)
                ctx.addIssue({
                    code: 'custom',
                    path: ['columns'],
                    message: 'a table built-in needs columns',
                })
            if (b.storage !== 'rows')
                ctx.addIssue({
                    code: 'custom',
                    path: ['storage'],
                    message: 'a table built-in keeps its rows in the host table',
                })
        } else if (b.columns) {
            ctx.addIssue({
                code: 'custom',
                path: ['columns'],
                message: 'only a table built-in has columns',
            })
        }
    })

/**
 * The host's default form: its pages, a page and width per built-in (absent
 * means the first page, full width), an optional help text per built-in (the
 * seeded field's `description`), and its rules. It seeds the first default
 * form, and every new form an admin starts from Settings → Forms → New form
 * (use-form-admin.ts starts from `defaultDefinitionFor`). Mirrored in Go
 * (server/forms HostLayout). A page has no description, because the Go page
 * has none and both seeds must write the same definition.
 */
export const hostLayoutSchema = z.object({
    pages: z.array(z.object({ key: keySchema, title: z.string().trim().min(1).max(120) })).min(1),
    fields: z
        .record(
            keySchema,
            z.object({
                page: keySchema.optional(),
                width: z.enum(FIELD_WIDTHS).optional(),
                description: z.string().max(500).optional(),
            })
        )
        .default({}),
    rules: z.array(ruleSchema).default([]),
})
export type FormHostLayout = z.infer<typeof hostLayoutSchema>
const ruleListSchema = z.array(ruleSchema)

type IssueSink = (path: (string | number)[], message: string) => void

/** Every key a layout names must be a built-in of the host or a page of the layout. */
function layoutIssues(layout: FormHostLayout, builtins: Set<string>, report: IssueSink) {
    const pages = new Set<string>()
    layout.pages.forEach((p, i) => {
        if (pages.has(p.key)) report(['pages', i, 'key'], `duplicate layout page key ${p.key}`)
        pages.add(p.key)
    })
    for (const [key, placed] of Object.entries(layout.fields)) {
        if (!builtins.has(key)) report(['fields', key], `${key} is not a built-in field`)
        if (placed.page !== undefined && !pages.has(placed.page))
            report(['fields', key, 'page'], `${placed.page} is not a layout page`)
    }
    layout.rules.forEach((rule, ri) => {
        rule.when.forEach((c, ci) => {
            if (!builtins.has(c.field))
                report(['rules', ri, 'when', ci, 'field'], `${c.field} is not a built-in field`)
        })
        rule.actions.forEach((a, ai) => {
            if (a.field !== undefined && !builtins.has(a.field))
                report(['rules', ri, 'actions', ai, 'field'], `${a.field} is not a built-in field`)
            if ('page' in a && a.page !== undefined && !pages.has(a.page))
                report(['rules', ri, 'actions', ai, 'page'], `${a.page} is not a layout page`)
        })
    })
}

/**
 * The layout seeds forms, so the definition it seeds must pass the same
 * checks a saved form passes. Each issue is reported where the layout causes
 * it: a seeded field's index maps back to its built-in key.
 */
function seededIssues(host: FormHostDef, report: IssueSink) {
    const parsed = definitionSchema.safeParse(defaultDefinitionFor(host))
    const issues: DefinitionIssue[] = parsed.success
        ? validateDefinitionForHost(parsed.data, host)
        : parsed.error.issues.map(issue => ({
              path: issue.path.filter((p): p is string | number => typeof p !== 'symbol'),
              message: issue.message,
          }))
    for (const issue of issues) {
        const [head, index, ...rest] = issue.path
        const builtin = head === 'fields' && typeof index === 'number' ? host.builtin[index] : null
        report(builtin ? ['fields', builtin.key, ...rest] : issue.path, issue.message)
    }
}

export const hostDefSchema = z
    .object({
        key: z.string().regex(/^[a-z][a-z0-9-]*:[a-z][a-z0-9_]*$/, 'host key is <slug>:<name>'),
        label: z.string().min(1).max(60),
        collection: z.string().min(1),
        formColumn: z.string().min(1),
        fieldsColumn: z.string().min(1),
        /** Every column of the collection, so a custom key can never shadow one. */
        columns: z.array(z.string().min(1)),
        builtin: z.array(builtinSchema),
        /** The custom kinds this host allows; absent means every kind. */
        kinds: z.array(z.enum(CUSTOM_KINDS)).optional(),
        files: z
            .object({
                collection: z.string().min(1),
                recordColumn: z.string().min(1),
                fieldColumn: z.string().min(1),
                sizeColumn: z.string().min(1),
                uploaderColumn: z.string().min(1),
                /** Holds the file's display name. */
                nameColumn: z.string().min(1).default('name'),
                /** The PocketBase file field. */
                fileColumn: z.string().min(1).default('file'),
                /** Attachment column ← host record column, copied onto every upload (a required `project`, say). Scalar columns only; an array or object source column is skipped. */
                copyColumns: z.record(z.string().min(1), z.string().min(1)).default({}),
            })
            .optional(),
        collaborative: z
            .object({
                fragment: z.string().includes('{record}').includes('{key}').includes('{format}'),
            })
            .optional(),
        /** Built-in key a new custom field lands before; absent means the page end. */
        insertCustomBefore: z.string().min(1).optional(),
        /** The first seeded default form's layout; absent means one full-width page. */
        layout: hostLayoutSchema.optional(),
    })
    .superRefine((host, ctx) => {
        const keys = new Set<string>()
        host.builtin.forEach((b, i) => {
            if (keys.has(b.key))
                ctx.addIssue({
                    code: 'custom',
                    path: ['builtin', i, 'key'],
                    message: `duplicate built-in key ${b.key}`,
                })
            keys.add(b.key)
            if (b.collaborative && !host.collaborative)
                ctx.addIssue({
                    code: 'custom',
                    path: ['builtin', i, 'collaborative'],
                    message: 'collaborative builtins need a collaborative fragment template',
                })
        })
        const computeFields = host.builtin.map(b => ({
            key: b.key,
            kind: b.kind,
            expression: b.expression,
            columns: b.columns,
        }))
        for (const issue of computedIssues(computeFields, ['builtin']))
            ctx.addIssue({ code: 'custom', path: issue.path, message: issue.message })
        if (host.insertCustomBefore && !keys.has(host.insertCustomBefore))
            ctx.addIssue({
                code: 'custom',
                path: ['insertCustomBefore'],
                message: 'insertCustomBefore must name a built-in key',
            })
        if (!host.layout) return
        const report: IssueSink = (path, message) =>
            ctx.addIssue({ code: 'custom', path: ['layout', ...path], message })
        layoutIssues(host.layout, keys, report)
        // zod runs this refinement on a host that already failed (an empty page
        // list, an unknown key), which the seeded form would only repeat or break on.
        if (ctx.issues.length === 0) seededIssues(host, report)
    })

/** Parsed host with defaults applied. Every consumer of parsed data uses these types. */
export type FormHostBuiltin = z.infer<typeof builtinSchema>
export type FormHostDef = z.infer<typeof hostDefSchema>
/** Hand-authored host module input, where defaulted fields may be omitted. */
export type FormHostDefInput = z.input<typeof hostDefSchema>
export const hostDefListSchema = z.array(hostDefSchema)

export function findBuiltin(host: FormHostDef, key: string): FormHostBuiltin | undefined {
    return host.builtin.find(b => b.key === key)
}

/**
 * The host's default form: the first seeded form and every new form started
 * from Settings → Forms → New form. Mirrored in Go (server/forms
 * defaultDefinition); both seeds write the same definition. Pages and rules
 * are copies, so an edit to one result never reaches the host or the next one.
 */
export function defaultDefinitionFor(host: FormHostDef): FormDefinition {
    const pages = host.layout?.pages.map(p => ({ key: p.key, title: p.title })) ?? [
        { key: 'main', title: host.label },
    ]
    const first = pages[0].key
    return {
        version: 1,
        pages,
        fields: host.builtin.map(b => {
            const placed = host.layout?.fields[b.key]
            return {
                key: b.key,
                kind: 'builtin',
                page: placed?.page ?? first,
                label: b.label,
                ...(placed?.description ? { description: placed.description } : {}),
                width: placed?.width ?? 'full',
                required: b.required,
                hidden: false,
            }
        }),
        // A zod parse builds new objects and arrays all the way down; Hermes has no structuredClone.
        rules: host.layout ? ruleListSchema.parse(host.layout.rules) : [],
        defaults: {},
        submit: { required: [] },
    }
}

export function resolveFragment(
    template: string,
    record: string,
    key: string,
    format: 'md' | 'html'
) {
    return template
        .replaceAll('{record}', () => record)
        .replaceAll('{key}', () => key)
        .replaceAll('{format}', () => format)
}
