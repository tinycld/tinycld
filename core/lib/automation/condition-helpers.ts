// Pure AST-surgery + operator-metadata helpers for the rule builder's
// condition pickers (TriggerCard/ConditionsCard/ConditionRow). No React —
// components stay thin wrappers around these.

import { newRecordId } from 'pbtsdb/core'
import type { CatalogField } from './api'
import { ALL_OPS, OPERATORS_BY_TYPE } from './helpers'
import type { ConditionOp } from './types'

// `uid` is a builder-local React key, never sent to the server: it exists
// solely so ConditionRow/ConditionGroupBox can key their list items on
// something stable instead of array index — deleting a middle row while a
// sibling's Menu is open must not reconcile that Menu onto the wrong row
// (Menu owns its own isOpen state internally, keyed by React identity).
// Optional so plain AST values (e.g. straight off the wire, pre-ensureUids)
// remain assignable. draftToRecord strips it via conditionsAstSchema.parse
// before the value is ever persisted.
export type Condition = {
    uid?: string
    field: string
    op: string
    value?: string | number | boolean
}
export type Group = { uid?: string; match: 'all' | 'any'; conditions: Condition[] }
export type ConditionsAst = { match: 'all' | 'any'; groups: Group[] }

const EMPTY_GROUP: Group = { match: 'all', conditions: [] }
const EMPTY_CONDITION: Condition = { field: '', op: '', value: undefined }

/**
 * Assigns a `uid` to every group/condition that's missing one, in place of
 * doing so ad hoc — a single pass callers run once after loading/creating a
 * draft (emptyDraft, recordToDraft) so every list item has a stable key from
 * the moment it first renders, not just the ones added via addGroup/addCondition.
 */
export function ensureUids(ast: ConditionsAst): ConditionsAst {
    return {
        ...ast,
        groups: ast.groups.map(g => ({
            ...g,
            uid: g.uid ?? newRecordId(),
            conditions: g.conditions.map(c => ({ ...c, uid: c.uid ?? newRecordId() })),
        })),
    }
}

/**
 * Legal operators for a field: `is_empty` doesn't apply to numbers (there's
 * no meaningful "empty" for a numeric column — it's either present or the
 * row wouldn't exist), so it's dropped from `OPERATORS_BY_TYPE.number`'s
 * result. Every other type returns its OPERATORS_BY_TYPE set unchanged.
 * Unknown/malformed field types (a version-skewed catalog) return [].
 */
export function operatorsForField(field: CatalogField): readonly ConditionOp[] {
    const ops = OPERATORS_BY_TYPE[field.type]
    if (!ops) return []
    if (field.type === 'number') return ops.filter(op => op !== 'is_empty')
    return ops
}

const OPERATOR_LABELS: Record<ConditionOp, string> = {
    contains: 'contains',
    not_contains: 'does not contain',
    equals: 'equals',
    starts_with: 'starts with',
    eq: '=',
    neq: '≠',
    gt: '>',
    lt: '<',
    is_true: 'is true',
    is_false: 'is false',
    before: 'is before',
    after: 'is after',
    within_last_days: 'is within the last (days)',
    is: 'is',
    is_not: 'is not',
    is_empty: 'is empty',
}

/** Human label for a condition operator. Covers every member of ALL_OPS. */
export function operatorLabel(op: string): string {
    return OPERATOR_LABELS[op as ConditionOp] ?? op
}

/** Appends a new empty OR-group. */
export function addGroup(ast: ConditionsAst): ConditionsAst {
    return {
        ...ast,
        groups: [...ast.groups, { ...EMPTY_GROUP, uid: newRecordId(), conditions: [] }],
    }
}

/** Removes the group at `groupIndex`. */
export function removeGroup(ast: ConditionsAst, groupIndex: number): ConditionsAst {
    return { ...ast, groups: ast.groups.filter((_, i) => i !== groupIndex) }
}

/** Toggles a group's own match mode (its conditions are all/any). */
export function setGroupMatch(
    ast: ConditionsAst,
    groupIndex: number,
    match: 'all' | 'any'
): ConditionsAst {
    return {
        ...ast,
        groups: ast.groups.map((g, i) => (i === groupIndex ? { ...g, match } : g)),
    }
}

/** Toggles the AST's top-level match mode (its groups are all/any). */
export function setTopMatch(ast: ConditionsAst, match: 'all' | 'any'): ConditionsAst {
    return { ...ast, match }
}

/** Appends a new empty condition to the group at `groupIndex`. */
export function addCondition(ast: ConditionsAst, groupIndex: number): ConditionsAst {
    return {
        ...ast,
        groups: ast.groups.map((g, i) =>
            i === groupIndex
                ? {
                      ...g,
                      conditions: [...g.conditions, { ...EMPTY_CONDITION, uid: newRecordId() }],
                  }
                : g
        ),
    }
}

/** Removes the condition at `conditionIndex` within the group at `groupIndex`. */
export function removeCondition(
    ast: ConditionsAst,
    groupIndex: number,
    conditionIndex: number
): ConditionsAst {
    return {
        ...ast,
        groups: ast.groups.map((g, i) =>
            i === groupIndex
                ? { ...g, conditions: g.conditions.filter((_, ci) => ci !== conditionIndex) }
                : g
        ),
    }
}

/** Immutably patches the condition at [groupIndex, conditionIndex]. */
export function updateCondition(
    ast: ConditionsAst,
    groupIndex: number,
    conditionIndex: number,
    patch: Partial<Condition>
): ConditionsAst {
    return {
        ...ast,
        groups: ast.groups.map((g, i) =>
            i === groupIndex
                ? {
                      ...g,
                      conditions: g.conditions.map((c, ci) =>
                          ci === conditionIndex ? { ...c, ...patch } : c
                      ),
                  }
                : g
        ),
    }
}

export { ALL_OPS }
