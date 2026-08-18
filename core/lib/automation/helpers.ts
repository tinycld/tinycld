import type { ConditionOp, FieldType } from './types'

export const OPERATORS_BY_TYPE: Record<FieldType, readonly ConditionOp[]> = {
    text: ['contains', 'not_contains', 'equals', 'starts_with'],
    number: ['eq', 'neq', 'gt', 'lt'],
    boolean: ['is_true', 'is_false'],
    date: ['before', 'after', 'within_last_days'],
    relation: ['is', 'is_not', 'is_empty'],
    select: ['is', 'is_not'],
}

export const ALL_OPS: readonly ConditionOp[] = [...new Set(Object.values(OPERATORS_BY_TYPE).flat())]

export const NO_VALUE_OPS: ReadonlySet<ConditionOp> = new Set(['is_true', 'is_false', 'is_empty'])

export function humanizeFieldKey(key: string): string {
    const spaced = key.replace(/_/g, ' ')
    return spaced.charAt(0).toUpperCase() + spaced.slice(1)
}

export function qualifyRef(pkgSlug: string, id: string): string {
    return `${pkgSlug}:${id}`
}

export function parseRef(ref: string): { pkg: string; id: string } {
    const idx = ref.indexOf(':')
    if (idx <= 0 || idx === ref.length - 1) {
        throw new Error(`malformed automation ref: '${ref}'`)
    }
    return { pkg: ref.slice(0, idx), id: ref.slice(idx + 1) }
}

/**
 * parseRef for RENDER paths, where throwing is the wrong failure.
 *
 * A rule's stored ref comes from the database — an older build, a hand-edited
 * row, or a package removed since it was written. Throwing during render blanks
 * the whole builder, so the user cannot even open the offending rule to fix or
 * delete it. Falling back to the raw ref shows something identifiable instead.
 */
export function parseRefSafe(ref: string): { pkg: string; id: string } {
    try {
        return parseRef(ref)
    } catch {
        return { pkg: '', id: ref }
    }
}
