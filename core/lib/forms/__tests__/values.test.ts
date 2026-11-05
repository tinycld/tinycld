import { describe, expect, it } from 'vitest'
import { z } from 'zod'
import { definitionSchema } from '../definition'
import { defaultDefinitionFor, hostDefSchema } from '../host'
import { evaluate } from '../logic'
import { buildSchema, type HostSchemas, isSubmittedKind, mergeValues, splitValues } from '../values'
import { widgetHost } from './fixtures'

const def = definitionSchema.parse({
    ...defaultDefinitionFor(widgetHost),
    fields: [
        ...defaultDefinitionFor(widgetHost).fields,
        {
            key: 'severity',
            kind: 'select',
            page: 'main',
            label: 'Severity',
            options: [
                { value: 'low', label: 'Low' },
                { value: 'high', label: 'High' },
            ],
        },
        { key: 'count', kind: 'number', page: 'main', label: 'Count', min: 0, max: 10 },
        { key: 'extra', kind: 'checklist', page: 'main', label: 'Extra' },
        { key: 'shot', kind: 'file', page: 'main', label: 'Shot', multiple: true },
    ],
    rules: [
        {
            key: 'r',
            when: [{ field: 'severity', op: 'is', value: 'high' }],
            actions: [{ action: 'require', field: 'count' }],
        },
    ],
})
const ctx = { today: '2026-01-01', userId: 'u' }

describe('buildSchema', () => {
    it('requires a required builtin and a rule-required field', () => {
        const state = evaluate(def, { severity: 'high' }, ctx)
        const r = buildSchema(def, widgetHost, state).safeParse({ severity: 'high' })
        expect(r.success).toBe(false)
        const paths = r.error?.issues.map(i => i.path.join('.'))
        expect(paths).toContain('name')
        expect(paths).toContain('count')
    })
    it('does not require a field hidden by a rule state', () => {
        const state = evaluate(def, { severity: 'low' }, ctx)
        const r = buildSchema(def, widgetHost, state).safeParse({ name: 'n' })
        expect(r.success).toBe(true)
    })
    it('rejects a select value outside the options and a number outside min max', () => {
        const state = evaluate(def, {}, ctx)
        const r = buildSchema(def, widgetHost, state).safeParse({
            name: 'n',
            severity: 'nope',
            count: 11,
        })
        const paths = r.error?.issues.map(i => i.path.join('.'))
        expect(paths).toEqual(expect.arrayContaining(['severity', 'count']))
    })
    it('skips sections, rows builtins and collaborative fields', () => {
        const state = evaluate(def, {}, ctx)
        const shape = buildSchema(def, widgetHost, state)
        expect(shape.safeParse({ name: 'n', comments: 'x', tasks: 'y', notes: 'z' }).success).toBe(
            true
        )
    })
})

describe('splitValues', () => {
    it('routes builtin columns, custom json and rows', () => {
        const out = splitValues(widgetHost, def, {
            name: 'N',
            due: '2026-01-02',
            severity: 'low',
            count: 2,
            tasks: [{ id: 't1', text: 'a', done: false }],
            extra: [{ id: 'e1', text: 'b', done: true }],
            comments: 'ignored',
        })
        expect(out).toEqual({
            columns: { name: 'N', due: '2026-01-02' },
            fields: { severity: 'low', count: 2, extra: [{ id: 'e1', text: 'b', done: true }] },
            rows: { tasks: [{ id: 't1', text: 'a', done: false }] },
            tables: {},
        })
    })
    it('omits undefined values but keeps null', () => {
        const out = splitValues(widgetHost, def, { name: 'N', due: undefined, severity: null })
        expect(out.columns).toEqual({ name: 'N' })
        expect(out.fields).toEqual({ severity: null })
    })
})

describe('mergeValues', () => {
    it('reads columns and the json column back into one values object', () => {
        const record = {
            id: 'w1',
            name: 'N',
            notes: '',
            due: '2026-01-02',
            owner: 'u',
            position: 'a0',
            form: 'f1',
            fields: { severity: 'low', count: 2 },
        }
        const values = mergeValues(widgetHost, def, record, {
            tasks: [{ id: 't1', text: 'a', done: false }],
        })
        expect(values).toEqual({
            name: 'N',
            notes: '',
            due: '2026-01-02',
            owner: 'u',
            severity: 'low',
            count: 2,
            tasks: [{ id: 't1', text: 'a', done: false }],
        })
    })
    it('tolerates a null json column', () => {
        const record = { id: 'w1', name: 'N', fields: null }
        expect(mergeValues(widgetHost, def, record)).toMatchObject({ name: 'N' })
    })
})

const wide = definitionSchema.parse({
    ...defaultDefinitionFor(widgetHost),
    fields: [
        ...defaultDefinitionFor(widgetHost).fields,
        {
            key: 'severity',
            kind: 'select',
            page: 'main',
            label: 'Severity',
            options: [
                { value: 'low', label: 'Low' },
                { value: 'high', label: 'High' },
            ],
        },
        {
            key: 'count',
            kind: 'number',
            page: 'main',
            label: 'Count',
            min: 0,
            max: 10,
            required: true,
        },
        { key: 'nick', kind: 'text', page: 'main', label: 'Nick' },
        { key: 'extra', kind: 'checklist', page: 'main', label: 'Extra' },
        {
            key: 'tags',
            kind: 'select',
            page: 'main',
            label: 'Tags',
            multiple: true,
            options: [
                { value: 'a', label: 'A' },
                { value: 'b', label: 'B' },
            ],
        },
        { key: 'flag', kind: 'toggle', page: 'main', label: 'Flag', required: true },
    ],
    rules: [
        {
            key: 'hide',
            when: [{ field: 'severity', op: 'is', value: 'low' }],
            actions: [{ action: 'hide', field: 'count' }],
        },
        {
            key: 'narrow',
            when: [{ field: 'name', op: 'eq', value: 'x' }],
            actions: [
                {
                    action: 'set_options',
                    field: 'severity',
                    options: [{ value: 'low', label: 'Low' }],
                },
            ],
        },
    ],
})
const base = { name: 'n', count: 2, flag: true }

describe('buildSchema, hidden and passthrough', () => {
    it('keeps hidden and row values in the output and does not validate them', () => {
        const schema = buildSchema(wide, widgetHost, evaluate(wide, { severity: 'low' }, ctx))
        const r = schema.safeParse({
            name: 'n',
            severity: 'low',
            count: 3,
            flag: true,
            tasks: [{ id: 't1', text: 'a', done: false }],
        })
        expect(r.success).toBe(true)
        expect(r.data).toMatchObject({ count: 3, tasks: [{ id: 't1', text: 'a', done: false }] })
        expect(
            schema.safeParse({ name: 'n', severity: 'low', count: 'x', flag: true }).success
        ).toBe(true)
    })
    it('does not require a field hidden by a rule even when it is required', () => {
        const low = buildSchema(wide, widgetHost, evaluate(wide, { severity: 'low' }, ctx))
        expect(low.safeParse({ name: 'n', severity: 'low', flag: true }).success).toBe(true)
        const high = buildSchema(wide, widgetHost, evaluate(wide, { severity: 'high' }, ctx))
        const r = high.safeParse({ name: 'n', severity: 'high', flag: true })
        expect(r.error?.issues.map(i => i.path.join('.'))).toContain('count')
    })
    it('accepts null for an empty text or date column and an empty json field', () => {
        const values = mergeValues(widgetHost, wide, {
            id: 'w1',
            name: 'N',
            due: null,
            notes: null,
            owner: null,
            nick: null,
            fields: { count: 2, flag: true },
        })
        expect(
            buildSchema(wide, widgetHost, evaluate(wide, values, ctx)).safeParse(values).success
        ).toBe(true)
    })
})

describe('buildSchema, rule-driven options', () => {
    it('narrows select values to the options a set_options rule sets', () => {
        const schema = buildSchema(wide, widgetHost, evaluate(wide, { name: 'x' }, ctx))
        expect(schema.safeParse({ ...base, name: 'x', severity: 'high' }).success).toBe(false)
        expect(schema.safeParse({ ...base, name: 'x', severity: 'low' }).success).toBe(true)
    })
    it('rejects every select value when a set_options list is empty', () => {
        const state = evaluate(wide, {}, ctx)
        const noOptions = {
            ...state,
            fields: { ...state.fields, severity: { ...state.fields.severity, options: [] } },
        }
        const schema = buildSchema(wide, widgetHost, noOptions)
        expect(schema.safeParse({ ...base, severity: 'low' }).success).toBe(false)
        expect(schema.safeParse(base).success).toBe(true)
    })
})

describe('buildSchema, field kinds', () => {
    const schema = buildSchema(wide, widgetHost, evaluate(wide, {}, ctx))
    it('validates a multiple select against its options and accepts an empty list', () => {
        expect(schema.safeParse({ ...base, tags: ['zzz'] }).success).toBe(false)
        expect(schema.safeParse({ ...base, tags: [] }).success).toBe(true)
        expect(schema.safeParse({ ...base, tags: ['a'] }).success).toBe(true)
    })
    it('rejects false for a required toggle', () => {
        const r = schema.safeParse({ ...base, flag: false })
        expect(r.error?.issues.map(i => i.path.join('.'))).toContain('flag')
        expect(schema.safeParse({ ...base, flag: true }).success).toBe(true)
    })
    it('validates checklist items', () => {
        expect(
            schema.safeParse({ ...base, extra: [{ id: 'e1', text: 'b', done: true }] }).success
        ).toBe(true)
        const r = schema.safeParse({ ...base, extra: [{ id: '', text: 'b', done: true }] })
        expect(r.error?.issues.map(i => i.path.join('.'))).toContain('extra.0.id')
    })
})

describe('isSubmittedKind', () => {
    const field = (key: string) => {
        const f = wide.fields.find(x => x.key === key)
        if (!f) throw new Error(`no field ${key}`)
        return f
    }
    it('excludes sections, row builtins and collaborative builtins', () => {
        expect(isSubmittedKind(widgetHost, field('comments'))).toBe(false)
        expect(isSubmittedKind(widgetHost, field('tasks'))).toBe(false)
        expect(isSubmittedKind(widgetHost, field('notes'))).toBe(false)
    })
    it('includes a plain column builtin', () => {
        expect(isSubmittedKind(widgetHost, field('name'))).toBe(true)
    })
})

describe('buildSchema, dates', () => {
    const dated = definitionSchema.parse({
        ...defaultDefinitionFor(widgetHost),
        fields: [
            ...defaultDefinitionFor(widgetHost).fields,
            { key: 'when', kind: 'date', page: 'main', label: 'When' },
            { key: 'at', kind: 'date', page: 'main', label: 'At', time: true, required: true },
        ],
    })
    const schema = buildSchema(dated, widgetHost, evaluate(dated, {}, ctx))
    const ok = { name: 'n', at: '2026-01-31T09:30' }
    it('accepts a day and a day with a time, the shape the server checks', () => {
        expect(schema.safeParse({ ...ok, when: '2026-01-31' }).success).toBe(true)
        expect(schema.safeParse({ ...ok, when: null }).success).toBe(true)
    })
    it.each([
        'tomorrow',
        '2026-1-31',
        '2026-01-31 09:30',
    ])('rejects %s in a custom date field', value => {
        const r = schema.safeParse({ ...ok, when: value })
        expect(r.error?.issues.map(i => i.path.join('.'))).toContain('when')
    })
    it('reports a missing required date as required', () => {
        const r = schema.safeParse({ name: 'n', at: '' })
        expect(r.error?.issues.find(i => i.path[0] === 'at')?.message).toBe('At is required')
    })
    it('leaves a built-in date column in the database format', () => {
        expect(schema.safeParse({ ...ok, due: '2026-01-31 00:00:00.000Z' }).success).toBe(true)
    })
})

describe('buildSchema messages', () => {
    const messageFor = (values: Record<string, unknown>, key: string) => {
        const state = evaluate(def, { severity: 'high', ...values }, ctx)
        const r = buildSchema(def, widgetHost, state).safeParse({ severity: 'high', ...values })
        return r.error?.issues.find(i => i.path.join('.') === key)?.message
    }
    it('says a required field is missing when it has no value at all', () => {
        // Never typed into: the form holds no value for it, not even ''.
        expect(messageFor({}, 'name')).toBe('Name is required')
        expect(messageFor({ name: '' }, 'name')).toBe('Name is required')
        expect(messageFor({ name: 'n', count: null }, 'count')).toBe('Count is required')
    })
    it('keeps the type message for a value of the wrong kind', () => {
        expect(messageFor({ name: 'n', count: 'many' }, 'count')).not.toBe('Count is required')
    })
})

describe('buildSchema, host component schemas', () => {
    const gadgetHost = hostDefSchema.parse({
        key: 'gizmos:gadget',
        label: 'Gadget',
        collection: 'gizmos_gadgets',
        formColumn: 'form',
        fieldsColumn: 'fields',
        columns: ['title', 'amount', 'label'],
        builtin: [
            { key: 'title', label: 'Title', kind: 'text', storage: 'column', required: true },
            {
                key: 'amount',
                label: 'Amount',
                kind: 'custom',
                storage: 'column',
                component: 'AmountField',
            },
            {
                key: 'label',
                label: 'Label',
                kind: 'text',
                storage: 'column',
                component: 'AmountField',
            },
        ],
    })
    const gadgetDef = defaultDefinitionFor(gadgetHost)
    const schemas: HostSchemas = { AmountField: z.number().min(0, 'Use 0 or more') }
    const schemaFor = (values: Record<string, unknown>, withSchemas?: HostSchemas) =>
        buildSchema(gadgetDef, gadgetHost, evaluate(gadgetDef, values, ctx), withSchemas)
    const issuesOf = (r: { error?: { issues: { path: PropertyKey[]; message: string }[] } }) =>
        r.error?.issues.map(i => ({ path: i.path, message: i.message }))

    it('validates a custom built-in with the schema of its component', () => {
        const refused = schemaFor({ title: 'T', amount: -1 }, schemas).safeParse({
            title: 'T',
            amount: -1,
        })
        expect(issuesOf(refused)).toEqual([{ path: ['amount'], message: 'Use 0 or more' }])
        expect(
            schemaFor({ title: 'T', amount: 5 }, schemas).safeParse({ title: 'T', amount: 5 })
                .success
        ).toBe(true)
        expect(schemaFor({ title: 'T' }, schemas).safeParse({ title: 'T' }).success).toBe(true)
    })
    it('falls back to any value when the host supplies no schema', () => {
        const values = { title: 'T', amount: 'x' }
        expect(schemaFor(values).safeParse(values).success).toBe(true)
        const refused = schemaFor(values, schemas).safeParse(values)
        expect(refused.error?.issues.map(i => i.path.join('.'))).toEqual(['amount'])
    })
    it('keeps the kind schema of a built-in that is not custom but names the component', () => {
        const values = { title: 'T', amount: -1, label: 3 }
        const textIssue = schemaFor(values)
            .safeParse(values)
            .error?.issues.find(i => i.path[0] === 'label')?.message
        expect(textIssue).toBeDefined()
        expect(issuesOf(schemaFor(values, schemas).safeParse(values))).toEqual([
            { path: ['amount'], message: 'Use 0 or more' },
            { path: ['label'], message: textIssue },
        ])
    })
    it('does not validate a custom built-in a rule hides', () => {
        const hiding = definitionSchema.parse({
            ...gadgetDef,
            rules: [
                {
                    key: 'hide_amount',
                    when: [{ field: 'title', op: 'eq', value: 'Hidden' }],
                    actions: [{ action: 'hide', field: 'amount' }],
                },
            ],
        })
        const parse = (values: Record<string, unknown>) =>
            buildSchema(hiding, gadgetHost, evaluate(hiding, values, ctx), schemas).safeParse(
                values
            )
        expect(parse({ title: 'Hidden', amount: -1 }).success).toBe(true)
        expect(issuesOf(parse({ title: 'Shown', amount: -1 }))).toEqual([
            { path: ['amount'], message: 'Use 0 or more' },
        ])
    })
    it('requires a custom built-in the state requires', () => {
        const required = definitionSchema.parse({ ...gadgetDef, submit: { required: ['amount'] } })
        const parse = (values: Record<string, unknown>) =>
            buildSchema(required, gadgetHost, evaluate(required, values, ctx), schemas).safeParse(
                values
            )
        expect(parse({ title: 'T' }).error?.issues.map(i => i.path.join('.'))).toEqual(['amount'])
        expect(issuesOf(parse({ title: 'T' }))).toEqual([
            { path: ['amount'], message: 'Amount is required' },
        ])
        expect(issuesOf(parse({ title: 'T', amount: -1 }))).toEqual([
            { path: ['amount'], message: 'Use 0 or more' },
        ])
    })
    it('refuses an empty required custom built-in with the missing text even when its schema accepts empty', () => {
        const required = definitionSchema.parse({ ...gadgetDef, submit: { required: ['amount'] } })
        const acceptsEmpty: HostSchemas = { AmountField: z.union([z.number(), z.literal('')]) }
        const values = { title: 'T', amount: '' }
        const refused = buildSchema(
            required,
            gadgetHost,
            evaluate(required, values, ctx),
            acceptsEmpty
        ).safeParse(values)
        expect(issuesOf(refused)).toEqual([{ path: ['amount'], message: 'Amount is required' }])
    })
    it('accepts null for a custom built-in that is not required', () => {
        const values = { title: 'T', amount: null }
        expect(schemaFor(values, schemas).safeParse(values).success).toBe(true)
    })
})

describe('buildSchema for a text built-in with a maxLength', () => {
    const plaque = hostDefSchema.parse({
        key: 'gizmos:plaque',
        label: 'Plaque',
        collection: 'gizmos_plaques',
        formColumn: 'form',
        fieldsColumn: 'fields',
        columns: ['serial', 'memo', 'form', 'fields'],
        builtin: [
            { key: 'serial', label: 'Serial', kind: 'text', storage: 'column', maxLength: 40 },
            { key: 'memo', label: 'Memo', kind: 'text', storage: 'column' },
        ],
    })
    const plaqueDef = definitionSchema.parse(defaultDefinitionFor(plaque))
    const schema = buildSchema(plaqueDef, plaque, evaluate(plaqueDef, {}, ctx))

    it('refuses one character over the limit on that built-in and accepts the limit', () => {
        const over = schema.safeParse({ serial: 'x'.repeat(41) })
        expect(over.success).toBe(false)
        expect(over.error?.issues.map(i => i.path.join('.'))).toEqual(['serial'])
        expect(schema.safeParse({ serial: 'x'.repeat(40) }).success).toBe(true)
    })

    it('puts no limit on a text built-in without a maxLength', () => {
        expect(schema.safeParse({ memo: 'x'.repeat(5000) }).success).toBe(true)
    })

    it('says the built-in is too long, as the server does', () => {
        const over = schema.safeParse({ serial: 'x'.repeat(41) })
        expect(over.error?.issues.find(i => i.path[0] === 'serial')?.message).toBe(
            'Serial is too long'
        )
    })
})

describe('buildSchema for custom text fields with a maxLength', () => {
    const lengthDef = definitionSchema.parse({
        ...defaultDefinitionFor(widgetHost),
        fields: [
            ...defaultDefinitionFor(widgetHost).fields,
            { key: 'nick', kind: 'text', page: 'main', label: 'Nick', maxLength: 10 },
            { key: 'bio', kind: 'textarea', page: 'main', label: 'Bio', maxLength: 20 },
        ],
    })
    const schema = buildSchema(lengthDef, widgetHost, evaluate(lengthDef, {}, ctx))
    const messageAt = (values: Record<string, unknown>, key: string) =>
        schema.safeParse({ name: 'n', ...values }).error?.issues.find(i => i.path[0] === key)
            ?.message

    it('says a custom text field over its maxLength is too long, as the server does', () => {
        expect(messageAt({ nick: 'x'.repeat(11) }, 'nick')).toBe('Nick is too long')
        expect(schema.safeParse({ name: 'n', nick: 'x'.repeat(10) }).success).toBe(true)
    })

    it('refuses a textarea value over its maxLength with the same message', () => {
        expect(messageAt({ bio: 'x'.repeat(21) }, 'bio')).toBe('Bio is too long')
        expect(schema.safeParse({ name: 'n', bio: 'x'.repeat(20) }).success).toBe(true)
    })
})
