// Pure AST-surgery + operator-metadata helpers for the rule builder's
// condition pickers (TriggerCard/ConditionsCard/ConditionRow). No React —
// components stay thin wrappers around these.

import type { CatalogField } from './api'
import { ALL_OPS, OPERATORS_BY_TYPE } from './helpers'
import type { ConditionOp } from './types'

type Condition = { field: string; op: string; value?: string | number | boolean }
type Group = { match: 'all' | 'any'; conditions: Condition[] }
type ConditionsAst = { match: 'all' | 'any'; groups: Group[] }

const EMPTY_GROUP: Group = { match: 'all', conditions: [] }
const EMPTY_CONDITION: Condition = { field: '', op: '', value: undefined }

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
    return { ...ast, groups: [...ast.groups, { ...EMPTY_GROUP, conditions: [] }] }
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
            i === groupIndex ? { ...g, conditions: [...g.conditions, { ...EMPTY_CONDITION }] } : g
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
