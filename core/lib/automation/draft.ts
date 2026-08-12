// Pure functions only — no React. The rule builder UI works against this
// draft shape, then converts to/from the PocketBase `rules` record on
// load/save. See `use-rule-mutations.ts` for the mutation-side glue.

import type { Rules } from '@tinycld/core/types/pbSchema'
import type { CatalogResponse } from './api'
import { ensureUids } from './condition-helpers'
import { OPERATORS_BY_TYPE } from './helpers'
import { conditionsAstSchema } from './schemas'

export interface RuleDraft {
    id?: string
    name: string
    scope: 'personal' | 'org'
    trigger: string // qualified ref; '' = unset
    triggerConfig: { cron?: string }
    conditions: ConditionsAstDraft
    actions: RuleDraftAction[]
    enabled: boolean
    stopProcessing: boolean
    order: number
}

// Re-derived locally (not imported from api.ts) to keep draft.ts's only
// runtime dependency on schemas.ts, per the module's "pure, no React" remit;
// the shape is identical to api.ts's ConditionsAst, widened with the
// builder-local `uid` condition-helpers.ts's ConditionsAst also carries (see
// that module for why: stable React keys for ConditionRow/ConditionGroupBox).
type ConditionsAstDraft = {
    match: 'all' | 'any'
    groups: {
        uid?: string
        match: 'all' | 'any'
        conditions: { uid?: string; field: string; op: string; value?: string | number | boolean }[]
    }[]
}

interface RuleDraftAction {
    ref: string
    params: Record<string, string | number | boolean>
}

const EMPTY_AST: ConditionsAstDraft = { match: 'all', groups: [] }

export type RulesRecordFields = Omit<Rules, 'id' | 'owner' | 'created' | 'updated'>

export function emptyDraft(scope: RuleDraft['scope']): RuleDraft {
    return {
        name: '',
        scope,
        trigger: '',
        triggerConfig: {},
        conditions: ensureUids(EMPTY_AST),
        actions: [],
        enabled: true,
        stopProcessing: false,
        order: 0,
    }
}

// The engine owns the writes, but a version-skewed or hand-edited record must
// never crash the builder — malformed JSON degrades to an empty AST/actions
// list rather than throwing. ensureUids runs on every load so a rule fetched
// from the server (whose stored conditions never carry a uid — draftToRecord
// strips it) gets stable React keys too, not just groups/conditions added
// fresh in this session via addGroup/addCondition.
function toConditionsAst(value: unknown): ConditionsAstDraft {
    const parsed = conditionsAstSchema.safeParse(value)
    return ensureUids(parsed.success ? parsed.data : EMPTY_AST)
}

function toActions(value: unknown): RuleDraftAction[] {
    if (!Array.isArray(value)) return []
    const actions: RuleDraftAction[] = []
    for (const item of value) {
        if (!item || typeof item !== 'object') continue
        const ref = (item as { ref?: unknown }).ref
        if (typeof ref !== 'string') continue
        const rawParams = (item as { params?: unknown }).params
        const params: Record<string, string | number | boolean> = {}
        if (rawParams && typeof rawParams === 'object') {
            for (const [key, v] of Object.entries(rawParams as Record<string, unknown>)) {
                if (typeof v === 'string' || typeof v === 'number' || typeof v === 'boolean') {
                    params[key] = v
                }
            }
        }
        actions.push({ ref, params })
    }
    return actions
}

function toTriggerConfig(value: unknown): { cron?: string } {
    if (!value || typeof value !== 'object') return {}
    const cron = (value as { cron?: unknown }).cron
    return typeof cron === 'string' ? { cron } : {}
}

export function recordToDraft(record: Rules): RuleDraft {
    return {
        id: record.id,
        name: record.name,
        scope: record.scope,
        trigger: record.trigger,
        triggerConfig: toTriggerConfig(record.trigger_config),
        conditions: toConditionsAst(record.conditions),
        actions: toActions(record.actions),
        enabled: record.enabled,
        stopProcessing: record.stop_processing,
        order: record.order,
    }
}

export function draftToRecord(draft: RuleDraft): RulesRecordFields {
    return {
        name: draft.name,
        scope: draft.scope,
        trigger: draft.trigger,
        trigger_config: draft.triggerConfig,
        // conditionsAstSchema has no `uid` field, so `.parse` strips it (zod's
        // default unknown-key behavior) — the builder-local React key never
        // reaches the server. The draft is always schema-valid by construction
        // (condition-helpers.ts only ever produces well-formed ASTs), so this
        // can't throw in practice; it's the serialization boundary, not a
        // validation gate — validateDraft already ran before save.
        conditions: conditionsAstSchema.parse(draft.conditions),
        actions: draft.actions,
        enabled: draft.enabled,
        stop_processing: draft.stopProcessing,
        order: draft.order,
    }
}

function findTrigger(catalog: CatalogResponse, ref: string) {
    return catalog.triggers.find(t => t.ref === ref)
}

function findAction(catalog: CatalogResponse, ref: string) {
    return catalog.actions.find(a => a.ref === ref)
}

function validateConditionsAgainstTrigger(
    conditions: RuleDraft['conditions'],
    trigger: CatalogResponse['triggers'][number]
): string[] {
    const errors: string[] = []
    const fieldsByKey = new Map((trigger.fields ?? []).map(f => [f.key, f]))
    for (const group of conditions.groups) {
        for (const condition of group.conditions) {
            const field = fieldsByKey.get(condition.field)
            if (!field) {
                errors.push(
                    `Condition field '${condition.field}' is not available on trigger '${trigger.label}'`
                )
                continue
            }
            const legalOps = OPERATORS_BY_TYPE[field.type]
            if (!legalOps.includes(condition.op as (typeof legalOps)[number])) {
                errors.push(
                    `Operator '${condition.op}' is not valid for field '${field.label}' (${field.type})`
                )
            }
        }
    }
    return errors
}

function validateTrigger(draft: RuleDraft, catalog: CatalogResponse): string[] {
    const trigger = findTrigger(catalog, draft.trigger)
    if (!trigger) return [`Trigger '${draft.trigger}' is not in the catalog`]

    const errors: string[] = []
    const isSynthetic = Boolean(trigger.synthetic)
    const hasConditions = draft.conditions.groups.some(g => g.conditions.length > 0)
    if (isSynthetic && hasConditions) {
        errors.push(`Trigger '${trigger.label}' has no fields — it cannot have conditions`)
    }
    if (!isSynthetic) {
        errors.push(...validateConditionsAgainstTrigger(draft.conditions, trigger))
    }
    if (trigger.synthetic === 'schedule' && !draft.triggerConfig.cron?.trim()) {
        errors.push('Schedule trigger requires a cron expression')
    }
    return errors
}

function validateActionParams(
    draftAction: RuleDraft['actions'][number],
    action: CatalogResponse['actions'][number]
): string[] {
    const errors: string[] = []
    for (const param of action.params ?? []) {
        // Text params and field-ref params may legitimately be empty strings
        // (matches what the engine can execute), so only relation params are
        // enforced non-empty here.
        if (param.field.type !== 'relation') continue
        const value = draftAction.params[param.key]
        const provided = value !== undefined && value !== null
        if (!provided || (typeof value === 'string' && value.trim() === '')) {
            errors.push(`Action '${action.label}' requires '${param.label}'`)
        }
    }
    return errors
}

function validateActionTarget(
    action: CatalogResponse['actions'][number],
    trigger: CatalogResponse['triggers'][number] | undefined
): string[] {
    if (action.opTarget !== 'trigger-record') return []
    if (!trigger || trigger.synthetic) {
        return [
            `Action '${action.label}' targets the trigger-record, but a synthetic trigger has no record to act on`,
        ]
    }
    if (trigger.collection !== action.collection) {
        return [
            `Action '${action.label}' targets collection '${action.collection}', which doesn't match trigger '${trigger.label}''s collection '${trigger.collection}'`,
        ]
    }
    return []
}

function validateAction(
    draftAction: RuleDraft['actions'][number],
    catalog: CatalogResponse,
    trigger: CatalogResponse['triggers'][number] | undefined
): string[] {
    const action = findAction(catalog, draftAction.ref)
    if (!action) return [`Action '${draftAction.ref}' is not in the catalog`]

    const errors: string[] = []
    if (!action.available) errors.push(`Action '${action.label}' is not available`)
    errors.push(...validateActionTarget(action, trigger))
    errors.push(...validateActionParams(draftAction, action))
    return errors
}

function validateActions(draft: RuleDraft, catalog: CatalogResponse): string[] {
    const trigger = findTrigger(catalog, draft.trigger)
    return draft.actions.flatMap(draftAction => validateAction(draftAction, catalog, trigger))
}

export function validateDraft(draft: RuleDraft, catalog: CatalogResponse | undefined): string[] {
    const errors: string[] = []

    if (!draft.name.trim()) errors.push('Name is required')

    if (!draft.trigger) {
        errors.push('Trigger is required')
    } else if (catalog) {
        errors.push(...validateTrigger(draft, catalog))
    }

    if (draft.actions.length === 0) {
        errors.push('At least one action is required')
    } else if (catalog) {
        errors.push(...validateActions(draft, catalog))
    }

    return errors
}
