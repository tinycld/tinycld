import { describe, expect, it } from 'vitest'
import { definitionSchema } from '../../forms/definition'
import { extendCatalogForDraft, FORM_SUBMITTED_TRIGGER, formTriggerFields } from '../form-trigger'

const def = definitionSchema.parse({
    version: 1,
    pages: [{ key: 'main', title: 'Main' }],
    fields: [
        { key: 'name', page: 'main', label: 'Name', kind: 'text' },
        { key: 'age', page: 'main', label: 'Age', kind: 'number' },
        { key: 'ok', page: 'main', label: 'OK', kind: 'toggle' },
        { key: 'when', page: 'main', label: 'When', kind: 'date' },
        {
            key: 'role',
            page: 'main',
            label: 'Role',
            kind: 'select',
            options: [{ value: 'dev', label: 'Developer' }],
        },
        { key: 'cv', page: 'main', label: 'CV', kind: 'file' },
        { key: 'todo', page: 'main', label: 'To do', kind: 'checklist' },
    ],
})

const catalog = {
    triggers: [
        {
            ref: FORM_SUBMITTED_TRIGGER,
            pkg: 'core',
            label: 'A web form is sent',
            collection: 'form_submissions',
            fields: [{ key: 'created', label: 'Sent at', type: 'date' as const }],
        },
        { ref: 'core:manual', pkg: 'core', label: 'Run manually', synthetic: 'manual' as const },
    ],
    actions: [],
}

describe('form trigger fields', () => {
    it('maps each answer kind to a catalog field under fields.', () => {
        expect(formTriggerFields(def)).toEqual([
            { key: 'fields.name', label: 'Name', type: 'text' },
            { key: 'fields.age', label: 'Age', type: 'number' },
            { key: 'fields.ok', label: 'OK', type: 'boolean' },
            { key: 'fields.when', label: 'When', type: 'date' },
            { key: 'fields.role', label: 'Role', type: 'select', options: ['dev'] },
            { key: 'fields.cv', label: 'CV (file names)', type: 'text' },
        ])
    })

    it('adds the form fields to the form trigger only', () => {
        const draft = { trigger: FORM_SUBMITTED_TRIGGER, triggerConfig: { form: 'f1' } }
        const out = extendCatalogForDraft(catalog, draft, def)
        const keys = out.triggers
            .find(t => t.ref === FORM_SUBMITTED_TRIGGER)
            ?.fields?.map(f => f.key)
        expect(keys?.[0]).toBe('created')
        expect(keys).toContain('fields.name')
        expect(out.triggers.find(t => t.ref === 'core:manual')?.fields).toBeUndefined()
    })

    it('leaves the catalog alone with no form picked', () => {
        expect(
            extendCatalogForDraft(
                catalog,
                { trigger: FORM_SUBMITTED_TRIGGER, triggerConfig: {} },
                undefined
            )
        ).toBe(catalog)
    })
})
