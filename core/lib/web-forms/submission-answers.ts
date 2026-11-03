import type { FormDefinition, FormField } from '@tinycld/core/lib/forms/definition'

export type Answer = { key: string; label: string; text: string }

// Files are listed apart (with a download), and a built-in has no answer.
const SKIPPED_KINDS = new Set(['builtin', 'file'])

function optionLabel(field: FormField, value: unknown): string {
    if (field.kind !== 'select') return String(value)
    return field.options.find(o => o.value === value)?.label ?? String(value)
}

function answerText(field: FormField, value: unknown): string {
    if (value === undefined || value === null || value === '') return ''
    if (typeof value === 'boolean') return value ? 'Yes' : 'No'
    if (field.kind === 'table' && Array.isArray(value)) return `${value.length} rows`
    if (Array.isArray(value))
        return value
            .map(v => optionLabel(field, typeof v === 'object' ? JSON.stringify(v) : v))
            .join(', ')
    if (typeof value === 'object') return JSON.stringify(value)
    return optionLabel(field, value)
}

function valuesOf(values: unknown): Record<string, unknown> {
    if (!values || typeof values !== 'object') return {}
    return values as Record<string, unknown>
}

/** Each answer of a submission as label and text, in the form's order. */
export function answersOf(definition: FormDefinition, values: unknown): Answer[] {
    const byKey = valuesOf(values)
    const out: Answer[] = []
    for (const field of definition.fields) {
        if (SKIPPED_KINDS.has(field.kind)) continue
        out.push({ key: field.key, label: field.label, text: answerText(field, byKey[field.key]) })
    }
    return out
}

/** The first two text answers, for a submission's row in a list. */
export function previewOf(definition: FormDefinition, values: unknown): string {
    const byKey = valuesOf(values)
    const texts: string[] = []
    for (const field of definition.fields) {
        if (field.kind !== 'text' && field.kind !== 'textarea') continue
        const value = byKey[field.key]
        if (typeof value === 'string' && value.trim()) texts.push(value.trim())
        if (texts.length === 2) break
    }
    return texts.join(' · ')
}
