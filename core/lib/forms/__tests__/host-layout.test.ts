import { describe, expect, it } from 'vitest'
import { definitionSchema, validateDefinitionForHost } from '../definition'
import { defaultDefinitionFor, hostDefSchema } from '../host'
import { widgetHost } from './fixtures'

const gadgetBase = {
    key: 'gizmos:gadget',
    label: 'Gadget',
    collection: 'gizmos_gadgets',
    formColumn: 'form',
    fieldsColumn: 'fields',
    columns: ['name', 'first', 'last', 'due', 'notes', 'form', 'fields'],
    builtin: [
        { key: 'name', label: 'Name', kind: 'text', storage: 'column', required: true },
        { key: 'first', label: 'First', kind: 'text', storage: 'column' },
        { key: 'last', label: 'Last', kind: 'text', storage: 'column' },
        { key: 'due', label: 'Due', kind: 'date', storage: 'column' },
        { key: 'notes', label: 'Notes', kind: 'textarea', storage: 'column' },
    ],
}

const hideDue = {
    key: 'hide_due',
    match: 'all',
    when: [{ field: 'name', op: 'is_empty' }],
    actions: [{ action: 'hide', field: 'due' }],
}

const gadgetLayout = {
    pages: [
        { key: 'details', title: 'Details' },
        { key: 'more', title: 'More' },
    ],
    fields: { first: { width: 'half' }, last: { width: 'half' }, notes: { page: 'more' } },
    rules: [hideDue],
}

const gadgetHost = hostDefSchema.parse({ ...gadgetBase, layout: gadgetLayout })

/** The paths of the issues a host with this layout is refused with. */
function layoutIssuePaths(layout: unknown): string[] {
    const parsed = hostDefSchema.safeParse({ ...gadgetBase, layout })
    expect(parsed.success).toBe(false)
    return (parsed.error?.issues ?? []).map(issue => issue.path.join('.'))
}

describe('a form host without a layout', () => {
    it('seeds one full-width page named after the host, as before', () => {
        expect(defaultDefinitionFor(widgetHost)).toEqual({
            version: 1,
            pages: [{ key: 'main', title: 'Widget' }],
            fields: [
                {
                    key: 'name',
                    kind: 'builtin',
                    page: 'main',
                    label: 'Name',
                    width: 'full',
                    required: true,
                    hidden: false,
                },
                {
                    key: 'notes',
                    kind: 'builtin',
                    page: 'main',
                    label: 'Notes',
                    width: 'full',
                    required: false,
                    hidden: false,
                },
                {
                    key: 'due',
                    kind: 'builtin',
                    page: 'main',
                    label: 'Due',
                    width: 'full',
                    required: false,
                    hidden: false,
                },
                {
                    key: 'owner',
                    kind: 'builtin',
                    page: 'main',
                    label: 'Owner',
                    width: 'full',
                    required: false,
                    hidden: false,
                },
                {
                    key: 'tasks',
                    kind: 'builtin',
                    page: 'main',
                    label: 'Tasks',
                    width: 'full',
                    required: false,
                    hidden: false,
                },
                {
                    key: 'comments',
                    kind: 'builtin',
                    page: 'main',
                    label: 'Comments',
                    width: 'full',
                    required: false,
                    hidden: false,
                },
            ],
            rules: [],
            defaults: {},
            submit: { required: [] },
        })
    })
})

describe('a form host with a layout', () => {
    it('seeds the layout pages in order, each built-in on its page and width, and the layout rules', () => {
        const def = defaultDefinitionFor(gadgetHost)
        expect(def.pages).toEqual([
            { key: 'details', title: 'Details' },
            { key: 'more', title: 'More' },
        ])
        expect(def.fields.map(f => [f.key, f.page, f.width])).toEqual([
            ['name', 'details', 'full'],
            ['first', 'details', 'half'],
            ['last', 'details', 'half'],
            ['due', 'details', 'full'],
            ['notes', 'more', 'full'],
        ])
        expect(def.fields.find(f => f.key === 'name')?.required).toBe(true)
        expect(def.rules).toEqual([hideDue])
    })

    it('seeds a definition that is valid for its host', () => {
        const def = definitionSchema.parse(defaultDefinitionFor(gadgetHost))
        expect(validateDefinitionForHost(def, gadgetHost)).toEqual([])
    })

    it('refuses a field entry that names no built-in', () => {
        expect(
            layoutIssuePaths({ ...gadgetLayout, fields: { ...gadgetLayout.fields, nope: {} } })
        ).toContain('layout.fields.nope')
    })

    it('refuses a field page that names no layout page', () => {
        expect(
            layoutIssuePaths({ ...gadgetLayout, fields: { notes: { page: 'elsewhere' } } })
        ).toContain('layout.fields.notes.page')
    })

    it('refuses two layout pages with one key', () => {
        expect(
            layoutIssuePaths({
                ...gadgetLayout,
                pages: [
                    { key: 'details', title: 'Details' },
                    { key: 'details', title: 'Again' },
                ],
                fields: {},
            })
        ).toContain('layout.pages.1.key')
    })

    it('refuses a width outside the field widths', () => {
        expect(
            layoutIssuePaths({ ...gadgetLayout, fields: { first: { width: 'wide' } } })
        ).toContain('layout.fields.first.width')
    })

    it('refuses a layout with no pages', () => {
        expect(layoutIssuePaths({ ...gadgetLayout, pages: [], fields: {} })).toContain(
            'layout.pages'
        )
    })

    it('refuses a rule condition on a field that is not a built-in', () => {
        const rule = { ...hideDue, when: [{ field: 'nope', op: 'is_empty' }] }
        expect(layoutIssuePaths({ ...gadgetLayout, rules: [rule] })).toContain(
            'layout.rules.0.when.0.field'
        )
    })

    it('refuses a rule action on a field that is not a built-in', () => {
        const rule = { ...hideDue, actions: [{ action: 'hide', field: 'nope' }] }
        expect(layoutIssuePaths({ ...gadgetLayout, rules: [rule] })).toContain(
            'layout.rules.0.actions.0.field'
        )
    })

    it('refuses a rule action on a page that is not a layout page', () => {
        const rule = { ...hideDue, actions: [{ action: 'show', page: 'elsewhere' }] }
        expect(layoutIssuePaths({ ...gadgetLayout, rules: [rule] })).toContain(
            'layout.rules.0.actions.0.page'
        )
    })
})

// The layout is the host's first seeded default form, so a layout whose seeded
// form definitionSchema or validateDefinitionForHost would refuse must itself
// be refused when the host is parsed, under `layout`.
describe('a form host whose layout would seed an invalid form', () => {
    it('refuses a rule that hides a required built-in', () => {
        const rule = { ...hideDue, actions: [{ action: 'hide', field: 'name' }] }
        expect(layoutIssuePaths({ ...gadgetLayout, rules: [rule] })).toContain(
            'layout.rules.0.actions.0.field'
        )
    })

    it('refuses a rule that hides the page holding a required built-in', () => {
        const rule = { ...hideDue, actions: [{ action: 'hide', page: 'details' }] }
        expect(layoutIssuePaths({ ...gadgetLayout, rules: [rule] })).toContain(
            'layout.rules.0.actions.0.page'
        )
    })

    it('refuses two rules with one key', () => {
        const again = { ...hideDue, when: [{ field: 'first', op: 'is_empty' }] }
        expect(layoutIssuePaths({ ...gadgetLayout, rules: [hideDue, again] })).toContain(
            'layout.rules.1.key'
        )
    })

    it('refuses a condition operator that does not suit the field kind', () => {
        const rule = { ...hideDue, when: [{ field: 'name', op: 'is_true' }] }
        expect(layoutIssuePaths({ ...gadgetLayout, rules: [rule] })).toContain(
            'layout.rules.0.when.0.op'
        )
    })

    it('refuses a text condition with no text value', () => {
        const rule = { ...hideDue, when: [{ field: 'name', op: 'contains' }] }
        expect(layoutIssuePaths({ ...gadgetLayout, rules: [rule] })).toContain(
            'layout.rules.0.when.0.value'
        )
    })
})

describe('the default definition of a host with a layout', () => {
    it('returns pages and rules of its own, so a change to one result does not reach the next', () => {
        const first = defaultDefinitionFor(gadgetHost)
        first.pages[0].title = 'Changed'
        first.pages.push({ key: 'extra', title: 'Extra' })
        first.rules[0].key = 'changed'
        first.rules[0].when.push({ field: 'first', op: 'is_empty' })
        first.rules[0].actions.push({ action: 'show', field: 'first' })
        first.rules.push(structuredClone(first.rules[0]))

        const second = defaultDefinitionFor(gadgetHost)
        expect(second.pages).toEqual(gadgetLayout.pages)
        expect(second.rules).toEqual([hideDue])
        expect(gadgetHost.layout?.pages).toEqual(gadgetLayout.pages)
        expect(gadgetHost.layout?.rules).toEqual([hideDue])
    })
})

describe('a form host layout field with a description', () => {
    const serialBase = {
        ...gadgetBase,
        columns: [...gadgetBase.columns, 'serial'],
        builtin: [
            ...gadgetBase.builtin,
            { key: 'serial', label: 'Serial', kind: 'text', storage: 'column' },
        ],
    }
    const serialLayout = {
        ...gadgetLayout,
        fields: { ...gadgetLayout.fields, serial: { description: 'Printed on the back.' } },
    }

    it('parses, and seeds the description on that field only', () => {
        const parsed = hostDefSchema.safeParse({ ...serialBase, layout: serialLayout })
        expect(parsed.error?.issues ?? []).toEqual([])
        const fields = defaultDefinitionFor(
            hostDefSchema.parse({ ...serialBase, layout: serialLayout })
        ).fields
        expect(fields.find(f => f.key === 'serial')?.description).toBe('Printed on the back.')
        expect(fields.filter(f => f.key !== 'serial').filter(f => 'description' in f)).toEqual([])
    })

    it('refuses a description longer than 500 characters', () => {
        const long = {
            ...serialLayout,
            fields: { ...gadgetLayout.fields, serial: { description: 'x'.repeat(501) } },
        }
        const parsed = hostDefSchema.safeParse({ ...serialBase, layout: long })
        expect(parsed.success).toBe(false)
        expect((parsed.error?.issues ?? []).map(issue => issue.path.join('.'))).toContain(
            'layout.fields.serial.description'
        )
    })
})

describe('a form host layout field with a fit-content or narrow width', () => {
    const sizedLayout = {
        ...gadgetLayout,
        fields: {
            ...gadgetLayout.fields,
            first: { width: 'quarter', narrowWidth: 'full' },
            due: { width: 'auto' },
        },
    }

    it('accepts auto and narrowWidth on a layout entry, and keeps them', () => {
        const parsed = hostDefSchema.safeParse({ ...gadgetBase, layout: sizedLayout })
        expect(parsed.error?.issues ?? []).toEqual([])
        expect(parsed.data?.layout?.fields.first).toEqual({ width: 'quarter', narrowWidth: 'full' })
        expect(parsed.data?.layout?.fields.due).toEqual({ width: 'auto' })
    })

    it('refuses an unknown narrow width', () => {
        expect(
            layoutIssuePaths({ ...gadgetLayout, fields: { first: { narrowWidth: 'wide' } } })
        ).toContain('layout.fields.first.narrowWidth')
    })

    it('parses a layout entry with no narrow width to no narrowWidth key', () => {
        const parsed = hostDefSchema.parse({ ...gadgetBase, layout: sizedLayout })
        expect('narrowWidth' in (parsed.layout?.fields.due ?? {})).toBe(false)
        expect('narrowWidth' in (parsed.layout?.fields.last ?? {})).toBe(false)
    })

    it('seeds the narrow width on its field only, and the fit-content width', () => {
        const fields = defaultDefinitionFor(
            hostDefSchema.parse({ ...gadgetBase, layout: sizedLayout })
        ).fields
        expect(fields.find(f => f.key === 'first')).toMatchObject({
            width: 'quarter',
            narrowWidth: 'full',
        })
        expect(fields.find(f => f.key === 'due')).toMatchObject({ width: 'auto' })
        expect(fields.filter(f => f.key !== 'first').filter(f => 'narrowWidth' in f)).toEqual([])
    })

    it('seeds a definition that is valid for its host', () => {
        const host = hostDefSchema.parse({ ...gadgetBase, layout: sizedLayout })
        const parsed = definitionSchema.safeParse(defaultDefinitionFor(host))
        expect(parsed.error?.issues ?? []).toEqual([])
        expect(parsed.data?.fields.find(f => f.key === 'first')?.narrowWidth).toBe('full')
        expect(parsed.success && validateDefinitionForHost(parsed.data, host)).toEqual([])
    })
})
