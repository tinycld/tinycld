import type {
    CustomField,
    CustomKind,
    FormDefinition,
    FormDefinitionInput,
    FormField,
    TableColumn,
} from '../definition'
import { definitionSchema, type TABLE_COLUMN_KINDS } from '../definition'
import { keyFromLabel, uniqueKey } from './key'

/** The editor's working shape: a definition whose rules may still be empty. */
export type DraftDefinition = Omit<FormDefinitionInput, 'fields'> & {
    fields: (FormDefinitionInput['fields'][number] & { acceptText?: string })[]
}
type RuleAction = NonNullable<DraftDefinition['rules']>[number]['actions'][number]

export interface Removed {
    rules: string[]
    actions: number
    defaults: boolean
    submitRequired: boolean
}

export class PageNotEmpty extends Error {
    constructor(key: string) {
        super(`page ${key} still has fields`)
    }
}

/**
 * Turns a draft into a definition the renderer can consume. Rules are dropped
 * because a draft's rules may be incomplete; the schema fills field defaults.
 * Throws a ZodError when the draft is invalid for reasons other than its rules.
 */
export function normalizeDraft(draft: DraftDefinition): FormDefinition {
    return definitionSchema.parse({ ...draft, rules: [], defaults: {}, submit: { required: [] } })
}

export function newCustomField(
    kind: CustomKind,
    label: string,
    page: string,
    taken: Set<string>
): CustomField {
    const base = {
        key: uniqueKey(keyFromLabel(label), taken),
        page,
        label: label.trim() || 'Field',
        width: 'full' as const,
        required: false,
        hidden: false,
    }
    switch (kind) {
        case 'select':
            return {
                ...base,
                kind,
                options: [{ value: 'option_1', label: 'Option 1' }],
                display: 'dropdown',
                multiple: false,
            }
        case 'user':
            return { ...base, kind, multiple: false }
        case 'date':
            return { ...base, kind, time: false }
        case 'file':
            return { ...base, kind, multiple: false, accept: [], maxSizeMb: 10 }
        case 'computed':
            return { ...base, kind, expression: 0, decimals: 0 }
        case 'table':
            return {
                ...base,
                kind,
                columns: [
                    { key: 'item', kind: 'text', label: 'Item', required: false, hidden: false },
                ],
            }
        case 'richtext':
        case 'markdown':
            return { ...base, kind, collaborative: false }
        default:
            return { ...base, kind }
    }
}

export function addField(def: DraftDefinition, field: FormField, index?: number): DraftDefinition {
    const fields = [...def.fields]
    fields.splice(index ?? fields.length, 0, field)
    return { ...def, fields }
}

function actionTargets(action: RuleAction, key: string): boolean {
    return 'field' in action && action.field === key
}

export function referencesOf(def: DraftDefinition, key: string): Removed {
    const out: Removed = {
        rules: [],
        actions: 0,
        defaults: Object.hasOwn(def.defaults ?? {}, key),
        submitRequired: (def.submit?.required ?? []).includes(key),
    }
    for (const r of def.rules ?? []) {
        const reads = r.when.some(c => c.field === key)
        const targeted = r.actions.filter(a => actionTargets(a, key)).length
        if (reads || (targeted > 0 && targeted === r.actions.length)) out.rules.push(r.key)
        else out.actions += targeted
    }
    return out
}

function stripReferences(
    def: DraftDefinition,
    predicateRule: (ruleKey: string) => boolean,
    predicateAction: (a: RuleAction) => boolean
): DraftDefinition {
    const rules = (def.rules ?? [])
        .filter(r => !predicateRule(r.key))
        .map(r => ({ ...r, actions: r.actions.filter(a => !predicateAction(a)) }))
    return { ...def, rules }
}

export function removeField(
    def: DraftDefinition,
    key: string
): { definition: DraftDefinition; removed: Removed } {
    const field = def.fields.find(f => f.key === key)
    if (!field)
        return {
            definition: def,
            removed: { rules: [], actions: 0, defaults: false, submitRequired: false },
        }
    if (field.kind === 'builtin')
        throw new Error(`${key} is a built-in field and cannot be removed`)
    const removed = referencesOf(def, key)
    const { [key]: _dropped, ...defaults } = def.defaults ?? {}
    const stripped = stripReferences(
        def,
        k => removed.rules.includes(k),
        a => actionTargets(a, key)
    )
    return {
        definition: {
            ...stripped,
            fields: def.fields.filter(f => f.key !== key),
            defaults,
            submit: { required: (def.submit?.required ?? []).filter(k => k !== key) },
        },
        removed,
    }
}

export function moveField(
    def: DraftDefinition,
    key: string,
    page: string,
    index: number
): DraftDefinition {
    const field = def.fields.find(f => f.key === key)
    if (!field) return def
    const others = def.fields.filter(f => f.key !== key)
    const onPage = others.filter(f => f.page === page)
    const anchor = onPage[Math.min(index, onPage.length)]
    const insertAt = anchor ? others.indexOf(anchor) : others.length
    const fields = [...others]
    fields.splice(insertAt, 0, { ...field, page })
    return { ...def, fields }
}

export function addPage(def: DraftDefinition, title: string): DraftDefinition {
    const key = uniqueKey(keyFromLabel(title), new Set(def.pages.map(p => p.key)))
    return { ...def, pages: [...def.pages, { key, title: title.trim() || 'Page' }] }
}

export function renamePage(def: DraftDefinition, key: string, title: string): DraftDefinition {
    const clean = title.trim() || 'Page'
    return { ...def, pages: def.pages.map(p => (p.key === key ? { ...p, title: clean } : p)) }
}

export function movePage(def: DraftDefinition, key: string, index: number): DraftDefinition {
    const page = def.pages.find(p => p.key === key)
    if (!page) return def
    const pages = def.pages.filter(p => p.key !== key)
    pages.splice(index, 0, page)
    return { ...def, pages }
}

export function removePage(
    def: DraftDefinition,
    key: string
): { definition: DraftDefinition; removed: Removed } {
    if (def.pages.length <= 1) throw new Error('cannot remove the last page')
    if (def.fields.some(f => f.page === key)) throw new PageNotEmpty(key)
    const targets = (a: RuleAction) => 'page' in a && a.page === key
    const rulesAllGone = (def.rules ?? [])
        .filter(r => r.actions.length > 0 && r.actions.every(targets))
        .map(r => r.key)
    const actions = (def.rules ?? []).reduce(
        (n, r) => n + (rulesAllGone.includes(r.key) ? 0 : r.actions.filter(targets).length),
        0
    )
    const stripped = stripReferences(def, k => rulesAllGone.includes(k), targets)
    return {
        definition: { ...stripped, pages: def.pages.filter(p => p.key !== key) },
        removed: { rules: rulesAllGone, actions, defaults: false, submitRequired: false },
    }
}

export function addRule(def: DraftDefinition): DraftDefinition {
    const key = uniqueKey('rule', new Set((def.rules ?? []).map(r => r.key)))
    return { ...def, rules: [...(def.rules ?? []), { key, match: 'all', when: [], actions: [] }] }
}

export function removeRule(def: DraftDefinition, key: string): DraftDefinition {
    return { ...def, rules: (def.rules ?? []).filter(r => r.key !== key) }
}

export type TableColumnKind = (typeof TABLE_COLUMN_KINDS)[number]

/**
 * A new column of a custom table. Its key comes from its label, unique among
 * its siblings and never `id`, which is the row's own; it never changes after.
 */
export function newTableColumn(
    kind: TableColumnKind,
    label: string,
    taken: Set<string>
): TableColumn {
    const base = {
        key: uniqueKey(keyFromLabel(label), new Set([...taken, 'id'])),
        label: label.trim() || 'Column',
        required: false,
        hidden: false,
    }
    switch (kind) {
        case 'select':
            return { ...base, kind, options: [{ value: 'option_1', label: 'Option 1' }] }
        case 'computed':
            return { ...base, kind, expression: 0, decimals: 0 }
        default:
            return { ...base, kind }
    }
}
