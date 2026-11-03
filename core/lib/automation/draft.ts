// Pure functions only — no React. The rule builder UI works against this
// draft shape, then converts to/from the PocketBase `rules` record on
// load/save. See `use-rule-mutations.ts` for the mutation-side glue and
// `rule-draft-schema.ts` for the builder's validation.

import type { Rules } from '@tinycld/core/types/pbSchema'
import type { ActionItem } from './condition-helpers'
import { ensureActionUids, ensureUids } from './condition-helpers'
import { conditionsAstSchema, ruleActionsSchema } from './schemas'

export interface RuleDraft {
    id?: string
    name: string
    scope: 'personal' | 'org'
    trigger: string // qualified ref; '' = unset
    triggerConfig: { cron?: string; form?: string }
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

// Re-derived from condition-helpers.ts's ActionItem, same reasoning as
// ConditionsAstDraft above: draft.ts stays pure/no-React while
// condition-helpers.ts's ensureActionUids works against the uid-bearing shape.
type RuleDraftAction = ActionItem

const EMPTY_AST: ConditionsAstDraft = { match: 'all', groups: [] }

export type RulesRecordFields = Omit<Rules, 'id' | 'owner' | 'created' | 'updated'>

export function emptyDraft(scope: RuleDraft['scope'], order = 0): RuleDraft {
    return {
        name: '',
        scope,
        trigger: '',
        triggerConfig: {},
        conditions: ensureUids(EMPTY_AST),
        actions: ensureActionUids([]),
        enabled: true,
        stopProcessing: false,
        order,
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
    // ensureActionUids runs on every load so a rule fetched from the server
    // (whose stored actions never carry a uid — draftToRecord strips it) has
    // the same shape as a fresh draft action.
    return ensureActionUids(actions)
}

function toTriggerConfig(value: unknown): RuleDraft['triggerConfig'] {
    if (!value || typeof value !== 'object') return {}
    const { cron, form } = value as { cron?: unknown; form?: unknown }
    return {
        ...(typeof cron === 'string' ? { cron } : {}),
        ...(typeof form === 'string' ? { form } : {}),
    }
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
        // validation gate — the builder's ruleDraftSchemaFor already ran before save.
        conditions: conditionsAstSchema.parse(draft.conditions),
        // Same uid-stripping as conditions above, via ruleActionsSchema (also
        // has no `uid` field). ruleActionsSchema.min(1) rejects an empty
        // array, so an empty draft.actions is passed through as `[]` directly
        // rather than through .parse — there's no uid to strip from nothing.
        actions: draft.actions.length > 0 ? ruleActionsSchema.parse(draft.actions) : [],
        enabled: draft.enabled,
        stop_processing: draft.stopProcessing,
        order: draft.order,
    }
}
