import { z } from 'zod'
import { type ComputeField, computedIssues } from './computed-validation'
import { computeExprSchema } from './expression'
import type { FormHostBuiltin, FormHostDef } from './host'
import { keySchema } from './key'

export const FIELD_WIDTHS = ['full', 'half', 'third', 'quarter'] as const
export type FieldWidth = (typeof FIELD_WIDTHS)[number]

export const CUSTOM_KINDS = [
    'text',
    'textarea',
    'number',
    'date',
    'toggle',
    'select',
    'user',
    'checklist',
    'richtext',
    'markdown',
    'file',
    'computed',
    'table',
] as const
export type CustomKind = (typeof CUSTOM_KINDS)[number]

export const KIND_NAMES: Record<CustomKind, string> = {
    text: 'Text',
    textarea: 'Long text',
    number: 'Number',
    date: 'Date',
    toggle: 'Toggle',
    select: 'Choice list',
    user: 'Person',
    checklist: 'Checklist',
    richtext: 'Rich text',
    markdown: 'Markdown',
    file: 'File',
    computed: 'Calculated value',
    table: 'Table',
}
export const FIELD_KINDS = [...CUSTOM_KINDS, 'builtin'] as const
export type FieldKind = (typeof FIELD_KINDS)[number]

export { keySchema }

export const CONDITION_OPS = [
    'eq',
    'neq',
    'contains',
    'starts_with',
    'is_empty',
    'not_empty',
    'gt',
    'lt',
    'gte',
    'lte',
    'before',
    'after',
    'is_true',
    'is_false',
    'is',
    'is_not',
    'is_any_of',
    'all_done',
    'any_open',
] as const
export type ConditionOp = (typeof CONDITION_OPS)[number]

/** Operators the builder offers and the schema accepts, by the kind a condition reads. */
export const OPERATORS_BY_KIND: Record<string, readonly ConditionOp[]> = {
    text: ['eq', 'neq', 'contains', 'starts_with', 'is_empty', 'not_empty'],
    textarea: ['eq', 'neq', 'contains', 'starts_with', 'is_empty', 'not_empty'],
    richtext: ['is_empty', 'not_empty'],
    markdown: ['is_empty', 'not_empty'],
    number: ['eq', 'neq', 'gt', 'lt', 'gte', 'lte', 'is_empty', 'not_empty'],
    date: ['before', 'after', 'is_empty', 'not_empty'],
    toggle: ['is_true', 'is_false'],
    select: ['is', 'is_not', 'is_any_of', 'is_empty', 'not_empty'],
    user: ['is', 'is_not', 'is_any_of', 'is_empty', 'not_empty'],
    checklist: ['all_done', 'any_open', 'is_empty'],
    file: ['is_empty', 'not_empty'],
    computed: ['eq', 'neq', 'gt', 'lt', 'gte', 'lte', 'is_empty', 'not_empty'],
    table: ['is_empty', 'not_empty'],
    custom: ['is_empty', 'not_empty'],
    section: [],
}

const optionSchema = z.object({
    value: z.string().min(1).max(100),
    label: z.string().min(1).max(120),
})
export type SelectOption = z.infer<typeof optionSchema>

const base = {
    key: keySchema,
    page: keySchema,
    label: z.string().trim().min(1).max(120),
    description: z.string().max(500).optional(),
    width: z.enum(FIELD_WIDTHS).default('full'),
    required: z.boolean().default(false),
    hidden: z.boolean().default(false),
    placeholder: z.string().max(200).optional(),
}

export const TABLE_COLUMN_KINDS = [
    'text',
    'number',
    'toggle',
    'date',
    'select',
    'computed',
] as const

const columnBase = {
    key: keySchema,
    label: z.string().trim().min(1).max(120),
    required: z.boolean().default(false),
    hidden: z.boolean().default(false),
    /**
     * Enter (or the native return key) in this cell of the table's last row
     * adds a row. Set it on the last editable cell of a row.
     */
    addOnSubmit: z.boolean().optional(),
}

export const columnSchema = z.discriminatedUnion('kind', [
    z.object({
        ...columnBase,
        kind: z.literal('text'),
        maxLength: z.number().int().positive().max(10000).optional(),
    }),
    z.object({
        ...columnBase,
        kind: z.literal('number'),
        min: z.number().optional(),
        max: z.number().optional(),
    }),
    z.object({ ...columnBase, kind: z.literal('toggle') }),
    z.object({ ...columnBase, kind: z.literal('date') }),
    z.object({
        ...columnBase,
        kind: z.literal('select'),
        options: z.array(optionSchema).min(1),
    }),
    z.object({
        ...columnBase,
        kind: z.literal('computed'),
        expression: computeExprSchema,
        decimals: z.number().int().min(0).max(6).default(0),
    }),
])
export type TableColumn = z.infer<typeof columnSchema>

/**
 * A host table built-in's column. It may name a component in the host's UI
 * module `components` map that draws its cell (a product picker, say). A custom
 * table column cannot: `columnSchema` strips the key, so a definition an admin
 * edits never mounts host code. The server never reads it.
 *
 * `flex` is the column's width weight on the wide grid (default 1); it is
 * host-only for the same reason, and presentation only.
 */
export const hostColumnSchema = columnSchema.and(
    z.object({
        component: z.string().min(1).optional(),
        flex: z.number().positive().max(12).optional(),
    })
)
export type HostTableColumn = z.infer<typeof hostColumnSchema>

const customFieldSchema = z.discriminatedUnion('kind', [
    z.object({
        ...base,
        kind: z.literal('text'),
        maxLength: z.number().int().positive().max(10000).optional(),
    }),
    z.object({
        ...base,
        kind: z.literal('textarea'),
        rows: z.number().int().min(1).max(40).optional(),
        maxLength: z.number().int().positive().max(10000).optional(),
    }),
    z.object({
        ...base,
        kind: z.literal('number'),
        min: z.number().optional(),
        max: z.number().optional(),
        step: z.number().positive().optional(),
    }),
    z.object({ ...base, kind: z.literal('date'), time: z.boolean().default(false) }),
    z.object({ ...base, kind: z.literal('toggle') }),
    z.object({
        ...base,
        kind: z.literal('select'),
        options: z.array(optionSchema).min(1),
        display: z.enum(['dropdown', 'radio', 'checkbox']).default('dropdown'),
        multiple: z.boolean().default(false),
    }),
    z.object({ ...base, kind: z.literal('user'), multiple: z.boolean().default(false) }),
    z.object({ ...base, kind: z.literal('checklist') }),
    z.object({ ...base, kind: z.literal('richtext'), collaborative: z.boolean().default(false) }),
    z.object({ ...base, kind: z.literal('markdown'), collaborative: z.boolean().default(false) }),
    z.object({
        ...base,
        kind: z.literal('file'),
        multiple: z.boolean().default(false),
        accept: z.array(z.string()).default([]),
        maxSizeMb: z.number().positive().max(100).default(10),
    }),
    z.object({
        ...base,
        kind: z.literal('computed'),
        expression: computeExprSchema,
        decimals: z.number().int().min(0).max(6).default(0),
    }),
    z.object({ ...base, kind: z.literal('table'), columns: z.array(columnSchema).min(1) }),
])
export type CustomField = z.infer<typeof customFieldSchema>

/** A host built-in placed on the form. Kind, storage and component come from the host at render time. */
const builtinRefSchema = z.object({ ...base, kind: z.literal('builtin') })
export type BuiltinFieldRef = z.infer<typeof builtinRefSchema>

// One discriminated union, not a union of two: zod then checks only the option the
// kind names, so a bad property is reported at its own path (fields.0.maxLength),
// not as "Invalid input" on the whole field.
const fieldSchema = z.discriminatedUnion('kind', [...customFieldSchema.options, builtinRefSchema])
export type FormField = z.infer<typeof fieldSchema>

const pageSchema = z.object({
    key: keySchema,
    title: z.string().trim().min(1).max(120),
    description: z.string().max(500).optional(),
})
export type FormPage = z.infer<typeof pageSchema>

/** The stored shape of a date value: a day, or a day and a time of day. Mirrored in Go (dateRe). */
export const DATE_RE = /^\d{4}-\d{2}-\d{2}(T\d{2}:\d{2})?$/
export const dateValueSchema = z.string().regex(DATE_RE, 'Use YYYY-MM-DD')

const todaySchema = z.strictObject({ today: z.tuple([]) })
/**
 * A default or set_value expression: a literal, or one of the picker's
 * expressions. Anything else would throw inside applyDefaults during render.
 * Mirrored in Go (validExpression).
 */
export const expressionSchema = z.union([
    z.string(),
    z.number(),
    z.boolean(),
    z.null(),
    z.array(z.string()),
    todaySchema,
    z.strictObject({ me: z.tuple([]) }),
    z.strictObject({
        addDays: z.tuple([z.union([dateValueSchema, todaySchema]), z.number().int()]),
    }),
])
export type FormExpression = z.infer<typeof expressionSchema>

const conditionValue = z.union([z.string(), z.number(), z.boolean(), z.array(z.string())])
const conditionSchema = z.object({
    field: keySchema,
    op: z.enum(CONDITION_OPS),
    value: conditionValue.optional(),
})
export type RuleCondition = z.infer<typeof conditionSchema>

const fieldTarget = z.object({ field: keySchema, page: z.undefined().optional() })
const pageTarget = z.object({ page: keySchema, field: z.undefined().optional() })
const target = z.union([fieldTarget, pageTarget])
const actionSchema = z.union([
    z.object({ action: z.literal('show') }).and(target),
    z.object({ action: z.literal('hide') }).and(target),
    z.object({ action: z.literal('require'), field: keySchema }),
    z.object({
        action: z.literal('set_options'),
        field: keySchema,
        options: z.array(optionSchema).min(1),
    }),
    z.object({
        action: z.literal('set_value'),
        field: keySchema,
        value: expressionSchema.default(null),
    }),
])
export type RuleAction = z.infer<typeof actionSchema>

export const ruleSchema = z.object({
    key: keySchema,
    match: z.enum(['all', 'any']).default('all'),
    when: z.array(conditionSchema).min(1, 'Add at least one condition'),
    actions: z.array(actionSchema).min(1, 'Add at least one action'),
})
export type FormRule = z.infer<typeof ruleSchema>

export const definitionSchema = z
    .object({
        version: z.literal(1),
        pages: z.array(pageSchema).min(1),
        fields: z.array(fieldSchema),
        rules: z.array(ruleSchema).default([]),
        /** key → literal or one of the picker's expressions */
        defaults: z.record(keySchema, expressionSchema).default({}),
        submit: z.object({ required: z.array(keySchema).default([]) }).default({ required: [] }),
    })
    .superRefine((def, ctx) => {
        // Mirrored in Go (duplicateOptionError).
        const duplicateOptions = (options: SelectOption[], path: (string | number)[]) => {
            const values = new Set<string>()
            options.forEach((o, j) => {
                if (values.has(o.value))
                    ctx.addIssue({
                        code: 'custom',
                        path: [...path, j, 'value'],
                        message: `duplicate option value ${o.value}`,
                    })
                values.add(o.value)
            })
        }
        const pages = new Set<string>()
        def.pages.forEach((p, i) => {
            if (pages.has(p.key))
                ctx.addIssue({
                    code: 'custom',
                    path: ['pages', i, 'key'],
                    message: `duplicate page key ${p.key}`,
                })
            pages.add(p.key)
        })
        const ruleKeys = new Set<string>()
        def.rules.forEach((r, i) => {
            if (ruleKeys.has(r.key))
                ctx.addIssue({
                    code: 'custom',
                    path: ['rules', i, 'key'],
                    message: `duplicate rule key ${r.key}`,
                })
            ruleKeys.add(r.key)
        })
        const kinds = new Map<string, string>()
        def.fields.forEach((f, i) => {
            if (kinds.has(f.key))
                ctx.addIssue({
                    code: 'custom',
                    path: ['fields', i, 'key'],
                    message: `duplicate field key ${f.key}`,
                })
            kinds.set(f.key, f.kind)
            if (f.kind === 'select') duplicateOptions(f.options, ['fields', i, 'options'])
            if (f.kind === 'table')
                f.columns.forEach((c, j) => {
                    if (c.kind === 'select')
                        duplicateOptions(c.options, ['fields', i, 'columns', j, 'options'])
                })
            if (f.kind === 'number' && f.min !== undefined && f.max !== undefined && f.min > f.max)
                ctx.addIssue({
                    code: 'custom',
                    path: ['fields', i, 'min'],
                    message: 'minimum is above maximum',
                })
            if (!pages.has(f.page))
                ctx.addIssue({
                    code: 'custom',
                    path: ['fields', i, 'page'],
                    message: `unknown page ${f.page}`,
                })
        })
        const computeFields: ComputeField[] = def.fields.map(f => {
            if (f.kind === 'computed') return { key: f.key, kind: f.kind, expression: f.expression }
            if (f.kind === 'table') return { key: f.key, kind: f.kind, columns: f.columns }
            return { key: f.key, kind: f.kind }
        })
        for (const issue of computedIssues(computeFields, ['fields']))
            ctx.addIssue({ code: 'custom', path: issue.path, message: issue.message })
        const requireField = (key: string, path: (string | number)[]) => {
            if (!kinds.has(key))
                ctx.addIssue({ code: 'custom', path, message: `unknown field ${key}` })
        }
        def.rules.forEach((r, ri) => {
            r.when.forEach((c, ci) => {
                requireField(c.field, ['rules', ri, 'when', ci, 'field'])
                const kind = kinds.get(c.field)
                // A builtin's real kind is only known with the host; validateDefinitionForHost rechecks it.
                if (kind && kind !== 'builtin' && !OPERATORS_BY_KIND[kind]?.includes(c.op)) {
                    ctx.addIssue({
                        code: 'custom',
                        path: ['rules', ri, 'when', ci, 'op'],
                        message: `${c.op} is not valid for a ${kind} field`,
                    })
                }
                const valuePath = ['rules', ri, 'when', ci, 'value']
                if ((c.op === 'contains' || c.op === 'starts_with') && typeof c.value !== 'string')
                    ctx.addIssue({
                        code: 'custom',
                        path: valuePath,
                        message: `${c.op} needs a text value`,
                    })
                if (
                    c.op === 'is_any_of' &&
                    !(
                        Array.isArray(c.value) &&
                        c.value.length > 0 &&
                        c.value.every(x => typeof x === 'string')
                    )
                )
                    ctx.addIssue({
                        code: 'custom',
                        path: valuePath,
                        message: 'is_any_of needs a list',
                    })
            })
            r.actions.forEach((a, ai) => {
                if (a.action === 'set_options')
                    duplicateOptions(a.options, ['rules', ri, 'actions', ai, 'options'])
                if ('field' in a && a.field)
                    requireField(a.field, ['rules', ri, 'actions', ai, 'field'])
                if ('page' in a && a.page && !pages.has(a.page)) {
                    ctx.addIssue({
                        code: 'custom',
                        path: ['rules', ri, 'actions', ai, 'page'],
                        message: `unknown page ${a.page}`,
                    })
                }
            })
        })
        for (const key of Object.keys(def.defaults)) requireField(key, ['defaults', key])
        def.submit.required.forEach((key, i) => {
            requireField(key, ['submit', 'required', i])
        })
    })

export type FormDefinition = z.infer<typeof definitionSchema>
export type FormDefinitionInput = z.input<typeof definitionSchema>

export interface DefinitionIssue {
    path: (string | number)[]
    message: string
}

/**
 * Checks a definition against the host it belongs to. Mirrored in Go (server/forms/definition.go).
 * Callers pass a definition that already passed `definitionSchema`; this function relies on it for
 * unique field keys, so "every builtin present exactly once" holds only after that check.
 */
export function validateDefinitionForHost(
    def: FormDefinition,
    host: FormHostDef
): DefinitionIssue[] {
    const issues: DefinitionIssue[] = []
    const builtins = new Map(host.builtin.map(b => [b.key, b]))
    const seen = new Set<string>()
    def.fields.forEach((f, i) => {
        if (f.kind === 'builtin') {
            const b = builtins.get(f.key)
            if (!b)
                issues.push({
                    path: ['fields', i, 'key'],
                    message: `${f.key} is not a built-in field of ${host.label}`,
                })
            else if (f.hidden && hideLock(b))
                issues.push({
                    path: ['fields', i, 'hidden'],
                    message: `${f.key} is ${hideLock(b)} by the record and cannot be hidden`,
                })
            seen.add(f.key)
            return
        }
        if (host.kinds && !host.kinds.includes(f.kind))
            issues.push({
                path: ['fields', i, 'kind'],
                message: `${KIND_NAMES[f.kind]} fields are not allowed in ${host.label}`,
            })
        if (builtins.has(f.key))
            issues.push({
                path: ['fields', i, 'key'],
                message: `${f.key} is a built-in field and cannot be a custom field`,
            })
        else if (host.columns.includes(f.key))
            issues.push({
                path: ['fields', i, 'key'],
                message: `${f.key} is a column of the record and cannot be a custom field`,
            })
        if ('collaborative' in f && f.collaborative && !host.collaborative) {
            issues.push({
                path: ['fields', i, 'collaborative'],
                message: `${host.label} does not support collaborative fields`,
            })
        }
    })
    for (const b of host.builtin)
        if (!seen.has(b.key))
            issues.push({ path: ['fields'], message: `missing built-in field ${b.key}` })
    issues.push(...hiddenRequiredIssues(def, host))
    def.rules.forEach((r, ri) => {
        r.when.forEach((c, ci) => {
            const b = builtins.get(c.field)
            if (b && !OPERATORS_BY_KIND[b.kind]?.includes(c.op)) {
                issues.push({
                    path: ['rules', ri, 'when', ci, 'op'],
                    message: `${c.op} is not valid for a ${b.kind} field`,
                })
            }
        })
    })
    issues.push(
        ...computedIssues(
            def.fields.map(f => computeFieldOf(f, builtins)),
            ['fields']
        )
    )
    return issues
}

function computeFieldOf(f: FormField, builtins: Map<string, FormHostBuiltin>): ComputeField {
    if (f.kind === 'builtin') {
        const b = builtins.get(f.key)
        return {
            key: f.key,
            kind: b?.kind ?? 'custom',
            expression: b?.expression,
            columns: b?.columns,
        }
    }
    if (f.kind === 'computed') return { key: f.key, kind: f.kind, expression: f.expression }
    if (f.kind === 'table') return { key: f.key, kind: f.kind, columns: f.columns }
    return { key: f.key, kind: f.kind }
}

/** Why a built-in may never be hidden: the record requires it, or it is locked. */
function hideLock(b: FormHostBuiltin | undefined): 'required' | 'locked' | null {
    if (b?.required) return 'required'
    if (b?.locked) return 'locked'
    return null
}

/**
 * A rule may not hide a built-in the host requires or locks, nor a page that
 * holds one. Mirrored in Go (actionError).
 */
function hiddenRequiredIssues(def: FormDefinition, host: FormHostDef): DefinitionIssue[] {
    const builtins = new Map(host.builtin.map(b => [b.key, b]))
    const heldOn = (page: string) =>
        def.fields.find(
            f => f.kind === 'builtin' && f.page === page && hideLock(builtins.get(f.key))
        )
    const issues: DefinitionIssue[] = []
    def.rules.forEach((r, ri) => {
        r.actions.forEach((a, ai) => {
            if (a.action !== 'hide') return
            const path = ['rules', ri, 'actions', ai]
            const held = a.page ? heldOn(a.page) : undefined
            if (held)
                issues.push({
                    path: [...path, 'page'],
                    message: `page ${a.page} holds ${hideLock(builtins.get(held.key))} field ${held.key}`,
                })
            const lock = a.field ? hideLock(builtins.get(a.field)) : null
            if (lock)
                issues.push({
                    path: [...path, 'field'],
                    message: `cannot hide ${lock} field ${a.field}`,
                })
        })
    })
    return issues
}
