import { describe, expect, it } from 'vitest'
import { validateDefinitionForHost } from '../definition'
import { defaultDefinitionFor, hostDefSchema, resolveFragment } from '../host'
import { widgetHost } from './fixtures'

describe('hostDefSchema', () => {
    it('accepts the widget host', () => {
        expect(hostDefSchema.safeParse(widgetHost).success).toBe(true)
    })
    it('accepts copyColumns on files and defaults it to empty', () => {
        const files = {
            collection: 'gizmos_files',
            recordColumn: 'widget',
            fieldColumn: 'field',
            sizeColumn: 'size',
            uploaderColumn: 'uploaded_by',
        }
        const parsed = hostDefSchema.parse({
            ...widgetHost,
            files: { ...files, copyColumns: { project: 'owner' } },
        })
        expect(parsed.files?.copyColumns).toEqual({ project: 'owner' })
        const bare = hostDefSchema.parse({ ...widgetHost, files })
        expect(bare.files?.copyColumns).toEqual({})
    })
    it('insertCustomBefore must name a built-in', () => {
        const r = hostDefSchema.safeParse({ ...widgetHost, insertCustomBefore: 'nope' })
        expect(r.success).toBe(false)
        expect(JSON.stringify(r.error?.issues)).toContain('insertCustomBefore')
        expect(
            hostDefSchema.parse({ ...widgetHost, insertCustomBefore: 'notes' }).insertCustomBefore
        ).toBe('notes')
    })
    it('rejects a key without a colon', () => {
        expect(hostDefSchema.safeParse({ ...widgetHost, key: 'widget' }).success).toBe(false)
    })
    it('accepts a rows builtin with no mapper', () => {
        expect(widgetHost.builtin.some(b => b.storage === 'rows')).toBe(true)
        expect(hostDefSchema.safeParse(widgetHost).success).toBe(true)
    })
    it('rejects a collaborative builtin when the host has no fragment template', () => {
        const r = hostDefSchema.safeParse({ ...widgetHost, collaborative: undefined })
        expect(r.success).toBe(false)
        expect(JSON.stringify(r.error?.issues)).toContain(
            'collaborative builtins need a collaborative fragment template'
        )
    })
    it('rejects a duplicate builtin key', () => {
        const r = hostDefSchema.safeParse({
            ...widgetHost,
            builtin: [...widgetHost.builtin, widgetHost.builtin[0]],
        })
        expect(r.success).toBe(false)
        expect(JSON.stringify(r.error?.issues)).toContain('duplicate built-in key name')
    })
    it('rejects a section with storage other than none', () => {
        const builtin = widgetHost.builtin.map(b =>
            b.key === 'comments' ? { ...b, storage: 'column' } : b
        )
        expect(hostDefSchema.safeParse({ ...widgetHost, builtin }).success).toBe(false)
    })
})

describe('defaultDefinitionFor', () => {
    it('places every builtin on one page in declared order', () => {
        const def = defaultDefinitionFor(widgetHost)
        expect(def.pages).toEqual([{ key: 'main', title: 'Widget' }])
        expect(def.fields.map(f => f.key)).toEqual([
            'name',
            'notes',
            'due',
            'owner',
            'tasks',
            'comments',
        ])
        expect(def.fields.every(f => f.kind === 'builtin' && f.page === 'main')).toBe(true)
        expect(def.fields[0]).toMatchObject({
            required: true,
            label: 'Name',
            width: 'full',
            hidden: false,
        })
    })
})

describe('validateDefinitionForHost', () => {
    const ok = defaultDefinitionFor(widgetHost)
    it('accepts the default definition', () => {
        expect(validateDefinitionForHost(ok, widgetHost)).toEqual([])
    })
    it('reports a missing builtin', () => {
        const def = { ...ok, fields: ok.fields.filter(f => f.key !== 'due') }
        expect(validateDefinitionForHost(def, widgetHost)).toEqual([
            { path: ['fields'], message: 'missing built-in field due' },
        ])
    })
    it('reports a custom key that collides with a host column', () => {
        const def = {
            ...ok,
            fields: [
                ...ok.fields,
                {
                    key: 'position',
                    kind: 'text' as const,
                    page: 'main',
                    label: 'P',
                    width: 'full' as const,
                    required: false,
                    hidden: false,
                },
            ],
        }
        expect(validateDefinitionForHost(def, widgetHost)[0]?.message).toBe(
            'position is a column of the record and cannot be a custom field'
        )
    })
    it('reports a hidden required builtin', () => {
        const def = {
            ...ok,
            fields: ok.fields.map(f => (f.key === 'name' ? { ...f, hidden: true } : f)),
        }
        expect(validateDefinitionForHost(def, widgetHost)[0]?.message).toBe(
            'name is required by the record and cannot be hidden'
        )
    })
    it('reports an operator the builtin kind does not support', () => {
        const def = {
            ...ok,
            rules: [
                {
                    key: 'r',
                    match: 'all' as const,
                    when: [{ field: 'due', op: 'contains' as const, value: 'x' }],
                    actions: [{ action: 'hide' as const, field: 'owner' }],
                },
            ],
        }
        expect(validateDefinitionForHost(def, widgetHost)[0]?.message).toBe(
            'contains is not valid for a date field'
        )
    })
    it('reports a builtin ref whose key is not a host builtin', () => {
        const def = {
            ...ok,
            fields: [
                ...ok.fields,
                {
                    key: 'ghost',
                    kind: 'builtin' as const,
                    page: 'main',
                    label: 'G',
                    width: 'full' as const,
                    required: false,
                    hidden: false,
                },
            ],
        }
        expect(validateDefinitionForHost(def, widgetHost)[0]?.message).toBe(
            'ghost is not a built-in field of Widget'
        )
    })
})

describe('hostDefSchema files', () => {
    it('defaults the name and file columns', () => {
        expect(widgetHost.files).toMatchObject({ nameColumn: 'name', fileColumn: 'file' })
        const parsed = hostDefSchema.parse({
            ...widgetHost,
            files: { ...widgetHost.files, nameColumn: 'title', fileColumn: 'blob' },
        })
        expect(parsed.files).toMatchObject({ nameColumn: 'title', fileColumn: 'blob' })
    })
})

describe('resolveFragment', () => {
    it('substitutes record, format and key', () => {
        expect(resolveFragment('widget:{record}:{format}:{key}', 'abc', 'notes', 'md')).toBe(
            'widget:abc:md:notes'
        )
    })
})

describe('host kinds and built-ins', () => {
    const base = {
        key: 'gizmos:note',
        label: 'Note',
        collection: 'gizmos_notes',
        formColumn: 'form',
        fieldsColumn: 'values',
        columns: ['id', 'form', 'values'],
    }

    it('accepts a host with no built-ins', () => {
        expect(hostDefSchema.safeParse({ ...base, builtin: [] }).success).toBe(true)
    })

    it('rejects a kinds entry that is not a custom kind', () => {
        const r = hostDefSchema.safeParse({ ...base, builtin: [], kinds: ['text', 'nope'] })
        expect(r.success).toBe(false)
    })
})

describe('a text built-in with a maxLength', () => {
    const plaque = {
        key: 'gizmos:plaque',
        label: 'Plaque',
        collection: 'gizmos_plaques',
        formColumn: 'form',
        fieldsColumn: 'fields',
        columns: ['serial', 'lit', 'form', 'fields'],
    }
    const serial = { key: 'serial', label: 'Serial', kind: 'text', storage: 'column' }
    const lit = { key: 'lit', label: 'Lit', kind: 'toggle', storage: 'column' }
    const issuePaths = (builtin: unknown[]) =>
        (hostDefSchema.safeParse({ ...plaque, builtin }).error?.issues ?? []).map(issue =>
            issue.path.join('.')
        )

    it('parses and keeps the limit on a text built-in', () => {
        const parsed = hostDefSchema.parse({
            ...plaque,
            builtin: [{ ...serial, maxLength: 40 }, lit],
        })
        expect(parsed.builtin[0].maxLength).toBe(40)
    })

    it('refuses a maxLength on a toggle built-in at that built-in', () => {
        const r = hostDefSchema.safeParse({
            ...plaque,
            builtin: [serial, { ...lit, maxLength: 40 }],
        })
        expect(r.success).toBe(false)
        expect(r.error?.issues).toEqual(
            expect.arrayContaining([
                expect.objectContaining({
                    path: ['builtin', 1, 'maxLength'],
                    message: 'maxLength is for a text built-in',
                }),
            ])
        )
    })

    it('refuses a maxLength of 0 and of 10001', () => {
        expect(issuePaths([{ ...serial, maxLength: 0 }])).toContain('builtin.0.maxLength')
        expect(issuePaths([{ ...serial, maxLength: 10001 }])).toContain('builtin.0.maxLength')
    })
})
