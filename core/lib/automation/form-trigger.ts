import type { FormDefinition } from '@tinycld/core/lib/forms/definition'
import type { CatalogField, CatalogResponse } from './api'
import type { RuleDraft } from './draft'

export const FORM_SUBMITTED_TRIGGER = 'core:form-submitted'

const TYPE_BY_KIND: Partial<Record<string, CatalogField['type']>> = {
    text: 'text',
    textarea: 'text',
    number: 'number',
    computed: 'number',
    toggle: 'boolean',
    date: 'date',
    select: 'select',
    file: 'text',
}

/** The answers a web form rule can read, as the server's fields resolver exposes them. */
export function formTriggerFields(definition: FormDefinition): CatalogField[] {
    const out: CatalogField[] = []
    for (const f of definition.fields) {
        const type = TYPE_BY_KIND[f.kind]
        if (!type) continue
        const label = f.kind === 'file' ? `${f.label} (file names)` : f.label
        const field: CatalogField = { key: `fields.${f.key}`, label, type }
        if (f.kind === 'select') field.options = f.options.map(o => o.value)
        out.push(field)
    }
    return out
}

/**
 * The catalog with the picked form's answers added to the form trigger, so
 * the condition editor, the template menu and the schema offer them with no
 * change of their own.
 */
export function extendCatalogForDraft(
    catalog: CatalogResponse,
    draft: Pick<RuleDraft, 'trigger' | 'triggerConfig'>,
    definition: FormDefinition | undefined
): CatalogResponse {
    if (draft.trigger !== FORM_SUBMITTED_TRIGGER || !definition) return catalog
    const extra = formTriggerFields(definition)
    return {
        ...catalog,
        triggers: catalog.triggers.map(t =>
            t.ref === FORM_SUBMITTED_TRIGGER ? { ...t, fields: [...(t.fields ?? []), ...extra] } : t
        ),
    }
}
