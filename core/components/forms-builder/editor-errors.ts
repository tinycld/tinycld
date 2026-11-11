import type { DraftDefinition } from '@tinycld/core/lib/forms/builder'

export interface ErrorLeaf {
    /** Dot path below the node the walk started at; '' for the node itself. */
    path: string
    type: string
    message: string
}

function isRecord(value: unknown): value is Record<string, unknown> {
    return typeof value === 'object' && value !== null
}

/** Every message in an error subtree, with its path; `ref` points at an element, not an error. */
export function errorLeaves(error: unknown, path = ''): ErrorLeaf[] {
    if (!isRecord(error)) return []
    const out: ErrorLeaf[] = []
    if (typeof error.message === 'string' && error.message) {
        const type = typeof error.type === 'string' ? error.type : 'custom'
        out.push({ path, type, message: error.message })
    }
    for (const [key, child] of Object.entries(error)) {
        if (key === 'ref' || key === 'message' || key === 'type' || key === 'types') continue
        out.push(...errorLeaves(child, path ? `${path}.${key}` : key))
    }
    return out
}

/** The first message in an error subtree. */
export function firstMessage(error: unknown): string | undefined {
    return errorLeaves(error)[0]?.message
}

const FIELD_PROPS: Record<string, string> = {
    label: 'label',
    key: 'key',
    page: 'page',
    description: 'description',
    placeholder: 'placeholder',
    width: 'width',
    narrowWidth: 'narrow width',
    required: 'required',
    hidden: 'hidden',
    options: 'options',
    display: 'display',
    multiple: 'several values',
    min: 'minimum',
    max: 'maximum',
    step: 'step',
    maxLength: 'maximum length',
    rows: 'rows',
    time: 'time',
    collaborative: 'collaborative editing',
    acceptText: 'accepted types',
    columns: 'columns',
    expression: 'formula',
    decimals: 'decimal places',
}

function fieldLabel(draft: DraftDefinition, index: number) {
    const field = draft.fields[index]
    return field ? `Field '${field.label}'` : `Field ${index + 1}`
}

function labelOfKey(draft: DraftDefinition, key: string) {
    const field = draft.fields.find(f => f.key === key)
    return `Field '${field?.label ?? key}'`
}

function columnLabel(draft: DraftDefinition, fieldIndex: number, columnIndex: number) {
    const field = draft.fields[fieldIndex]
    const column = field?.kind === 'table' ? field.columns[columnIndex] : undefined
    return column ? `column '${column.label}'` : `column ${columnIndex + 1}`
}

function describeRule(parts: string[]): string {
    const rule = `Rule ${Number(parts[1]) + 1}`
    const [, , list, item] = parts
    if (list === 'when')
        return item === undefined || item === 'root'
            ? `${rule}, conditions`
            : `${rule}, condition ${Number(item) + 1}`
    if (list === 'actions')
        return item === undefined || item === 'root'
            ? `${rule}, actions`
            : `${rule}, action ${Number(item) + 1}`
    return rule
}

/**
 * Where an editor error is, in words: "Rule 1, condition 2",
 * "Field 'Severity', options", "Field 'Parts', column 'Qty', label". An empty string means the form as a whole.
 */
export function describePath(path: string, draft: DraftDefinition): string {
    const parts = path.split('.')
    const [head, second, third] = parts
    if (head === '' || head === 'root') return ''
    if (head === 'fields') {
        if (second === undefined || second === 'root') return 'Fields'
        const field = fieldLabel(draft, Number(second))
        if (third === undefined) return field
        const [, , , column, prop] = parts
        if (third === 'columns' && column !== undefined && column !== 'root') {
            const where = `${field}, ${columnLabel(draft, Number(second), Number(column))}`
            return prop === undefined ? where : `${where}, ${FIELD_PROPS[prop] ?? prop}`
        }
        return `${field}, ${FIELD_PROPS[third] ?? third}`
    }
    if (head === 'pages') {
        const page = draft.pages[Number(second)]
        return page ? `Page '${page.title}'` : 'Pages'
    }
    if (head === 'rules') return second === undefined ? 'Rules' : describeRule(parts)
    if (head === 'defaults') return second ? `${labelOfKey(draft, second)}, default` : 'Defaults'
    if (head === 'submit') return 'Required on submit'
    return path
}
