import { describe, expect, it } from 'vitest'
import { definitionSchema, validateDefinitionForHost } from '../definition'
import { defaultDefinitionFor, hostDefSchema } from '../host'
import { bugDefinition, widgetHost } from './fixtures'

describe('definitionSchema', () => {
    it('accepts a valid definition and fills defaults', () => {
        const parsed = definitionSchema.parse({
            version: 1,
            pages: [{ key: 'p', title: 'P' }],
            fields: [],
        })
        expect(parsed.rules).toEqual([])
        expect(parsed.defaults).toEqual({})
        expect(parsed.submit.required).toEqual([])
    })
    it('accepts the bug definition', () => {
        expect(definitionSchema.safeParse(bugDefinition).success).toBe(true)
    })
    it('rejects a duplicate field key', () => {
        const r = definitionSchema.safeParse({
            ...bugDefinition,
            fields: [...bugDefinition.fields, { ...bugDefinition.fields[1] }],
        })
        expect(r.success).toBe(false)
        expect(JSON.stringify(r.error?.issues)).toContain('duplicate')
    })
    it('rejects a duplicate rule key', () => {
        const rules = [bugDefinition.rules[0], { ...bugDefinition.rules[0] }]
        const r = definitionSchema.safeParse({ ...bugDefinition, rules })
        expect(r.success).toBe(false)
        expect(JSON.stringify(r.error?.issues)).toContain('duplicate rule key r1')
    })
    it('rejects a duplicate option value within a select field', () => {
        const severity = {
            ...bugDefinition.fields[1],
            options: [
                { value: 'low', label: 'Low' },
                { value: 'low', label: 'Also low' },
            ],
        }
        const fields = [bugDefinition.fields[0], severity, bugDefinition.fields[2]]
        const r = definitionSchema.safeParse({ ...bugDefinition, fields })
        expect(r.success).toBe(false)
        expect(r.error?.issues).toContainEqual(
            expect.objectContaining({
                path: ['fields', 1, 'options', 1, 'value'],
                message: 'duplicate option value low',
            })
        )
    })
    it('rejects a duplicate option value within a set_options action', () => {
        const rule = {
            key: 'r9',
            match: 'all',
            when: [{ field: 'title', op: 'is_empty' }],
            actions: [
                {
                    action: 'set_options',
                    field: 'severity',
                    options: [
                        { value: 'a', label: 'A' },
                        { value: 'a', label: 'Also A' },
                    ],
                },
            ],
        }
        const r = definitionSchema.safeParse({ ...bugDefinition, rules: [rule] })
        expect(r.success).toBe(false)
        expect(r.error?.issues).toContainEqual(
            expect.objectContaining({
                path: ['rules', 0, 'actions', 0, 'options', 1, 'value'],
                message: 'duplicate option value a',
            })
        )
    })
    it('rejects a number field whose minimum is above its maximum', () => {
        const count = {
            key: 'count',
            kind: 'number',
            page: 'details',
            label: 'Count',
            min: 5,
            max: 2,
        }
        const at = (field: object) =>
            definitionSchema.safeParse({
                ...bugDefinition,
                fields: [...bugDefinition.fields, field],
            })
        const r = at(count)
        expect(r.success).toBe(false)
        expect(r.error?.issues).toContainEqual(
            expect.objectContaining({
                path: ['fields', bugDefinition.fields.length, 'min'],
                message: 'minimum is above maximum',
            })
        )
        expect(at({ ...count, max: 5 }).success).toBe(true)
    })
    it('rejects a key that is not a slug', () => {
        const bad = { ...bugDefinition.fields[1], key: 'Bad Key' }
        expect(
            definitionSchema.safeParse({ ...bugDefinition, fields: [bugDefinition.fields[0], bad] })
                .success
        ).toBe(false)
    })
    it('rejects a field on a page that does not exist', () => {
        const bad = { ...bugDefinition.fields[1], page: 'nope' }
        expect(
            definitionSchema.safeParse({ ...bugDefinition, fields: [bugDefinition.fields[0], bad] })
                .success
        ).toBe(false)
    })
    it('rejects a rule that names a missing field', () => {
        const rules = [
            { ...bugDefinition.rules[0], when: [{ field: 'ghost', op: 'is', value: 'x' }] },
        ]
        expect(definitionSchema.safeParse({ ...bugDefinition, rules }).success).toBe(false)
    })
    it('rejects a rule action that names a missing page', () => {
        const rules = [{ ...bugDefinition.rules[0], actions: [{ action: 'show', page: 'ghost' }] }]
        expect(definitionSchema.safeParse({ ...bugDefinition, rules }).success).toBe(false)
    })
    it('rejects an operator the field kind does not support', () => {
        const rules = [
            { ...bugDefinition.rules[0], when: [{ field: 'severity', op: 'gt', value: 1 }] },
        ]
        expect(definitionSchema.safeParse({ ...bugDefinition, rules }).success).toBe(false)
    })
    it('rejects contains with a non-text value', () => {
        const rules = [
            { ...bugDefinition.rules[0], when: [{ field: 'title', op: 'contains', value: 5 }] },
        ]
        const r = definitionSchema.safeParse({ ...bugDefinition, rules })
        expect(r.success).toBe(false)
        expect(r.error?.issues.map(i => i.message)).toContain('contains needs a text value')
    })
    it('rejects is_any_of without a list of text', () => {
        const rules = [
            { ...bugDefinition.rules[0], when: [{ field: 'kind', op: 'is_any_of', value: [] }] },
        ]
        const r = definitionSchema.safeParse({ ...bugDefinition, rules })
        expect(r.success).toBe(false)
        expect(r.error?.issues.map(i => i.message)).toContain('is_any_of needs a list')
    })
    it('rejects a require action on a page', () => {
        const rules = [
            { ...bugDefinition.rules[0], actions: [{ action: 'require', page: 'repro' }] },
        ]
        expect(definitionSchema.safeParse({ ...bugDefinition, rules }).success).toBe(false)
    })
    it('rejects a default for a missing field', () => {
        expect(
            definitionSchema.safeParse({ ...bugDefinition, defaults: { ghost: 1 } }).success
        ).toBe(false)
    })
    it('rejects an empty pages list', () => {
        expect(definitionSchema.safeParse({ ...bugDefinition, pages: [] }).success).toBe(false)
    })
})

describe('expressionSchema in defaults and set_value', () => {
    const withDefault = (expr: unknown) => ({ ...bugDefinition, defaults: { severity: expr } })
    const withSetValue = (expr: unknown) => ({
        ...bugDefinition,
        rules: [
            {
                ...bugDefinition.rules[0],
                actions: [{ action: 'set_value', field: 'severity', value: expr }],
            },
        ],
    })
    const valid = [
        '2026-01-31',
        3,
        true,
        null,
        ['a', 'b'],
        { today: [] },
        { me: [] },
        { addDays: [{ today: [] }, 21] },
        { addDays: ['2026-01-31', -3] },
    ]
    const invalid = [
        { var: 'x' },
        { today: [1] },
        { addDays: [{ me: [] }, 2] },
        { addDays: [{ today: [] }, 1.5] },
        { addDays: ['soon', 2] },
        { today: [], me: [] },
        [1],
        { addDays: [{ addDays: [{ today: [] }, 1] }, 1] },
    ]
    it.each(valid.map(v => [JSON.stringify(v), v]))('accepts %s', (_, expr) => {
        expect(definitionSchema.safeParse(withDefault(expr)).success).toBe(true)
        expect(definitionSchema.safeParse(withSetValue(expr)).success).toBe(true)
    })
    it.each(invalid.map(v => [JSON.stringify(v), v]))('rejects %s', (_, expr) => {
        expect(definitionSchema.safeParse(withDefault(expr)).success).toBe(false)
        expect(definitionSchema.safeParse(withSetValue(expr)).success).toBe(false)
    })
    it('reads a set_value with no value as null, like the server', () => {
        const parsed = definitionSchema.parse(withSetValue(undefined))
        expect(parsed.rules[0].actions[0]).toEqual({
            action: 'set_value',
            field: 'severity',
            value: null,
        })
    })
})

describe('validateDefinitionForHost, rules that hide', () => {
    const ok = defaultDefinitionFor(widgetHost)
    const twoPages = {
        ...ok,
        pages: [...ok.pages, { key: 'more', title: 'More' }],
    }
    const hiding = (
        action: { action: 'hide'; field: string } | { action: 'hide'; page: string }
    ) => ({
        ...twoPages,
        rules: [
            {
                key: 'r',
                match: 'all' as const,
                when: [{ field: 'due', op: 'is_empty' as const }],
                actions: [action],
            },
        ],
    })
    it('rejects a rule that hides a required built-in', () => {
        expect(
            validateDefinitionForHost(hiding({ action: 'hide', field: 'name' }), widgetHost)
        ).toEqual([
            {
                path: ['rules', 0, 'actions', 0, 'field'],
                message: 'cannot hide required field name',
            },
        ])
    })
    it('rejects a rule that hides the page holding a required built-in', () => {
        expect(
            validateDefinitionForHost(hiding({ action: 'hide', page: 'main' }), widgetHost)
        ).toEqual([
            {
                path: ['rules', 0, 'actions', 0, 'page'],
                message: 'page main holds required field name',
            },
        ])
    })
    it('accepts hiding a page or a field with no required built-in', () => {
        expect(
            validateDefinitionForHost(hiding({ action: 'hide', page: 'more' }), widgetHost)
        ).toEqual([])
        expect(
            validateDefinitionForHost(hiding({ action: 'hide', field: 'owner' }), widgetHost)
        ).toEqual([])
    })
})

describe('host kinds allowlist', () => {
    const host = hostDefSchema.parse({
        key: 'gizmos:note',
        label: 'Note',
        collection: 'gizmos_notes',
        formColumn: 'form',
        fieldsColumn: 'values',
        columns: ['id', 'form', 'values'],
        builtin: [],
        kinds: ['text', 'file'],
    })

    it('rejects a field whose kind the host does not allow', () => {
        const def = definitionSchema.parse({
            version: 1,
            pages: [{ key: 'main', title: 'Main' }],
            fields: [{ key: 'who', page: 'main', label: 'Who', kind: 'user' }],
        })
        expect(validateDefinitionForHost(def, host)).toEqual([
            { path: ['fields', 0, 'kind'], message: 'Person fields are not allowed in Note' },
        ])
    })

    it('defaults a file field to 10 MB', () => {
        const def = definitionSchema.parse({
            version: 1,
            pages: [{ key: 'main', title: 'Main' }],
            fields: [{ key: 'cv', page: 'main', label: 'CV', kind: 'file' }],
        })
        expect(def.fields[0]).toMatchObject({ kind: 'file', maxSizeMb: 10, accept: [] })
    })
})
