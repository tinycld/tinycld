// Pure AST-surgery + operator-metadata helpers for the rule builder's
// condition pickers (TriggerCard/ConditionsCard/ConditionRow). No React —
// components stay thin wrappers around these.

import { newRecordId } from 'pbtsdb/core'
import type { CatalogAction, CatalogField, CatalogResponse, CatalogTrigger } from './api'
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

// Same rationale as Condition/Group's `uid` above, applied to THEN actions:
// ActionsCard's up/down reorder (moveAction) actively remaps array indices,
// so an index key would reconcile an open param Menu or focused input onto
// whichever action slides into that slot. `uid` gives ActionEntry a stable
// React key that moves WITH its action. Optional so a plain action item
// (straight off the wire, pre-ensureActionUids) remains assignable.
// draftToRecord strips it via ruleActionsSchema.parse before persisting.
export type ActionItem = {
    uid?: string
    ref: string
    params: Record<string, string | number | boolean>
}

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
 * Assigns a `uid` to every action missing one — same one-pass-on-load
 * rationale as ensureUids, run by emptyDraft/recordToDraft so a rule loaded
 * from the server (whose stored actions never carry a uid — draftToRecord
 * strips it) gets stable keys too, not just actions appended fresh in this
 * session via ActionsCard's add-action Menu.
 */
export function ensureActionUids(actions: ActionItem[]): ActionItem[] {
    return actions.map(a => ({ ...a, uid: a.uid ?? newRecordId() }))
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

/**
 * The AST that the first edit to an empty condition set produces: one group
 * holding one condition carrying the user's field pick.
 *
 * ConditionsCard renders a ready-to-fill first row against `groups: []` WITHOUT
 * putting it in the draft, and calls this only once the user actually picks a
 * field. Keeping the blank row out of the draft is what lets an untouched
 * builder still save a rule with no conditions (which matches everything) — and
 * it is load-bearing, not just tidy: `conditionSchema.field` is `min(1)` and
 * `conditionGroupSchema.conditions` is `min(1)`, so a blank condition or an
 * empty group reaching `conditionsAstSchema.parse` in draftToRecord THROWS on
 * save.
 *
 * A patch that names no field is ignored for the same reason. That is currently
 * unreachable — ConditionRow's operator menu lists nothing until a field is
 * chosen — but the invariant lives in a different component, so this guards it
 * here rather than trusting a sibling's render logic to hold forever.
 */
export function seedFirstCondition(ast: ConditionsAst, patch: Partial<Condition>): ConditionsAst {
    if (!patch.field) return ast
    return {
        ...ast,
        groups: [
            {
                ...EMPTY_GROUP,
                uid: newRecordId(),
                conditions: [{ ...EMPTY_CONDITION, ...patch, uid: newRecordId() }],
            },
        ],
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

/**
 * Actions the THEN card may offer for the given trigger. A trigger-record op
 * (`opTarget === 'trigger-record'`) only makes sense against the record that
 * actually fired the trigger, so it's excluded unless its own `collection`
 * matches the trigger's — and excluded entirely for a synthetic trigger
 * (schedule/manual), which has no record at all. `create`-op record actions
 * and native actions don't touch the trigger record, so they're always
 * listed. Unavailable actions (`available: false`, e.g. an uninstalled
 * package) stay in the list — the caller renders them disabled with a "needs
 * {pkg}" suffix rather than hiding them outright.
 */
export function compatibleActions(
    catalog: CatalogResponse,
    trigger: CatalogTrigger
): CatalogAction[] {
    return catalog.actions.filter(action => {
        if (action.opTarget !== 'trigger-record') return true
        if (trigger.synthetic) return false
        return action.collection === trigger.collection
    })
}

/**
 * Immutably moves the item at `from` to index `to`. Out-of-bounds `to` is a
 * no-op (returns a value equal to `list`, not necessarily the same
 * reference) rather than clamping — an up/down button already can't request
 * an out-of-bounds move under normal use (it's disabled at the ends), so this
 * is a defensive guard, not a UI-reachable path.
 */
export function moveAction<T>(list: T[], from: number, to: number): T[] {
    if (to < 0 || to >= list.length) return list
    const next = [...list]
    const [item] = next.splice(from, 1)
    next.splice(to, 0, item)
    return next
}

/**
 * Orders the builder's package groups to match the order the user dragged their
 * apps into (Settings → Personal → Navigation), so the trigger and action menus
 * read the same way as the sidebar and tab bar rather than alphabetically.
 *
 * Two kinds of slug are deliberately NOT in `orderedSlugs`:
 *
 *   • `core`, which has no manifest and so never appears in the nav order at
 *     all. Its synthetic triggers (run manually / on a schedule) are the
 *     package-neutral starting points every rule can use, so they lead.
 *   • packages with no nav entry (settings-only contributors), which the nav
 *     order filters out. They still contribute automation, so they follow the
 *     ordered ones alphabetically instead of being dropped.
 */
export function orderGroupsByUserPreference<T>(
    groups: Map<string, T>,
    orderedSlugs: string[]
): [string, T][] {
    const rank = new Map(orderedSlugs.map((slug, i) => [slug, i]))
    const weight = (pkg: string) => {
        if (pkg === 'core') return -1
        return rank.get(pkg) ?? Number.MAX_SAFE_INTEGER
    }
    return [...groups.entries()].sort(([a], [b]) => {
        const delta = weight(a) - weight(b)
        // Equal weight means both are unranked; alphabetical keeps them stable
        // rather than leaving it to Map insertion order, which varies with the
        // catalog's own row order.
        return delta !== 0 ? delta : a.localeCompare(b)
    })
}

/** Appends a `{{key}}` template placeholder to a param's current text value. */
export function appendPlaceholder(value: string, key: string): string {
    return `${value}{{${key}}}`
}

/**
 * Splices a reordered subset back into the full ordered id list: subset
 * members keep their new relative order but occupy the same POSITIONS the
 * subset occupied in the full sequence, so ids outside the subset (e.g. rules
 * hidden by a package filter) are untouched. The caller renumbers the full
 * result 0..N-1 — this only fixes relative order, not the numbering.
 *
 * Ids present in the subset but absent from the full list are appended at
 * the end; this shouldn't happen in practice (the subset is always derived
 * from the full list) but is handled defensively rather than dropping data.
 */
export function mergeReorderedSubset(
    fullOrderedIds: string[],
    reorderedSubsetIds: string[]
): string[] {
    const subsetPositions = new Set(reorderedSubsetIds)
    let nextSubsetIndex = 0
    const merged = fullOrderedIds.map(id => {
        if (!subsetPositions.has(id)) return id
        return reorderedSubsetIds[nextSubsetIndex++]
    })
    const leftover = reorderedSubsetIds.slice(nextSubsetIndex)
    return [...merged, ...leftover]
}

export { ALL_OPS }
