// The rule builder's validation, as a zod schema for useForm's zodResolver.
// The catalog decides most of what a valid rule is (which fields a trigger
// exposes, which params an action requires), so the schema is built per
// catalog. Every issue carries the path of the draft value it is about.

import { z } from 'zod'
import type { CatalogAction, CatalogResponse, CatalogTrigger } from './api'
import type { RuleDraft } from './draft'
import { FORM_SUBMITTED_TRIGGER } from './form-trigger'
import { OPERATORS_BY_TYPE } from './helpers'
import type { ConditionOp } from './types'

type Issue = { message: string; path: (string | number)[] }

// The operators evalCondition reads with toFloat: eq/neq/gt/lt over a number
// field, plus within_last_days, whose value is a day count on a date field.
const NUMERIC_OPS: ReadonlySet<ConditionOp> = new Set(['eq', 'neq', 'gt', 'lt', 'within_last_days'])

function isFiniteValue(value: unknown): boolean {
    if (typeof value === 'number') return Number.isFinite(value)
    // A stored string is tolerated only if it actually reads as a number: rules
    // written by an older build (or by hand) may carry "5" rather than 5.
    if (typeof value === 'string' && value.trim() !== '') return Number.isFinite(Number(value))
    return false
}

function conditionIssues(conditions: RuleDraft['conditions'], trigger: CatalogTrigger): Issue[] {
    const issues: Issue[] = []
    const fieldsByKey = new Map((trigger.fields ?? []).map(f => [f.key, f]))
    conditions.groups.forEach((group, groupIndex) => {
        group.conditions.forEach((condition, conditionIndex) => {
            const path = ['conditions', 'groups', groupIndex, 'conditions', conditionIndex]
            const field = fieldsByKey.get(condition.field)
            if (!field) {
                issues.push({
                    message: `Condition field '${condition.field}' is not available on trigger '${trigger.label}'`,
                    path: [...path, 'field'],
                })
                return
            }
            const legalOps = OPERATORS_BY_TYPE[field.type]
            if (!legalOps.includes(condition.op as (typeof legalOps)[number])) {
                issues.push({
                    message: `Operator '${condition.op}' is not valid for field '${field.label}' (${field.type})`,
                    path: [...path, 'op'],
                })
            }
            if (NUMERIC_OPS.has(condition.op as ConditionOp) && !isFiniteValue(condition.value)) {
                // The engine's numeric operators fail closed on a value they
                // can't read as a number, so saving one is saving a rule that
                // silently never matches. Caught here rather than at the input.
                issues.push({
                    message: `Condition on '${field.label}' needs a number`,
                    path: [...path, 'value'],
                })
            }
        })
    })
    return issues
}

function triggerIssues(draft: RuleDraft, trigger: CatalogTrigger | undefined): Issue[] {
    if (!trigger) {
        return [{ message: `Trigger '${draft.trigger}' is not in the catalog`, path: ['trigger'] }]
    }

    const issues: Issue[] = []
    const isSynthetic = Boolean(trigger.synthetic)
    const hasConditions = draft.conditions.groups.some(g => g.conditions.length > 0)
    if (isSynthetic && hasConditions) {
        issues.push({
            message: `Trigger '${trigger.label}' has no fields — it cannot have conditions`,
            path: ['conditions'],
        })
    }
    if (!isSynthetic) {
        issues.push(...conditionIssues(draft.conditions, trigger))
    }
    if (trigger.synthetic === 'schedule' && !draft.triggerConfig.cron?.trim()) {
        issues.push({
            message: 'Schedule trigger requires a cron expression',
            path: ['triggerConfig', 'cron'],
        })
    }
    if (draft.trigger === FORM_SUBMITTED_TRIGGER && !draft.triggerConfig.form) {
        issues.push({
            message: 'Pick the form these submissions come from',
            path: ['triggerConfig', 'form'],
        })
    }
    return issues
}

function actionParamIssues(
    draftAction: RuleDraft['actions'][number],
    action: CatalogAction,
    actionIndex: number
): Issue[] {
    const issues: Issue[] = []
    for (const param of action.params ?? []) {
        // Most text and field-ref params may legitimately be empty strings, so
        // only params the catalog marks required are enforced — every relation
        // param, plus text params that back a required column.
        //
        // This used to test `type !== 'relation'` and skip everything else,
        // which let core:notify save with a blank title. The rule then RAN,
        // reported a matched run, and failed at the insert with "title: cannot
        // be blank" — a notification nobody received, on a run that looked
        // healthy.
        // A relation param is required regardless of the flag: the engine
        // refuses an empty one, and deriving it here means the client stays
        // correct against a catalog (or a test fixture) that predates
        // `required` rather than silently dropping the check.
        if (!param.required && param.field.type !== 'relation') continue
        const value = draftAction.params[param.key]
        const provided = value !== undefined && value !== null
        if (!provided || (typeof value === 'string' && value.trim() === '')) {
            issues.push({
                message: `Action '${action.label}' requires '${param.label}'`,
                path: ['actions', actionIndex, 'params', param.key],
            })
        }
    }
    return issues
}

function actionTargetMessage(action: CatalogAction, trigger: CatalogTrigger | undefined) {
    if (action.opTarget !== 'trigger-record') return null
    if (!trigger || trigger.synthetic) {
        return `Action '${action.label}' targets the trigger-record, but a synthetic trigger has no record to act on`
    }
    if (trigger.collection !== action.collection) {
        return `Action '${action.label}' targets collection '${action.collection}', which doesn't match trigger '${trigger.label}''s collection '${trigger.collection}'`
    }
    return null
}

// Action-level issues sit on the action itself (`actions.N`), not on its
// `ref`: react-hook-form keeps a field's DOM ref under an error's `ref` key, so
// an error filed at `actions.N.ref` is skipped by FormErrorSummary. They are
// emitted before the param issues so the resolver nests the params under them
// rather than overwriting them.
function actionIssues(
    draftAction: RuleDraft['actions'][number],
    actionIndex: number,
    catalog: CatalogResponse,
    trigger: CatalogTrigger | undefined
): Issue[] {
    const path = ['actions', actionIndex]
    const action = catalog.actions.find(a => a.ref === draftAction.ref)
    if (!action) return [{ message: `Action '${draftAction.ref}' is not in the catalog`, path }]

    const issues: Issue[] = []
    if (!action.available)
        issues.push({ message: `Action '${action.label}' is not available`, path })
    const targetMessage = actionTargetMessage(action, trigger)
    if (targetMessage) issues.push({ message: targetMessage, path })
    issues.push(...actionParamIssues(draftAction, action, actionIndex))
    return issues
}

function catalogIssues(draft: RuleDraft, catalog: CatalogResponse): Issue[] {
    const trigger = catalog.triggers.find(t => t.ref === draft.trigger)
    const issues: Issue[] = []
    if (draft.trigger) issues.push(...triggerIssues(draft, trigger))
    draft.actions.forEach((draftAction, index) => {
        issues.push(...actionIssues(draftAction, index, catalog, trigger))
    })
    return issues
}

/**
 * The rule builder's form schema. Without a catalog only the catalog-free
 * rules apply (a name, a trigger, at least one action).
 *
 * `conditions` and the action items are not re-validated structurally: the
 * builder only ever produces well-formed ones, and draftToRecord's own schemas
 * are the serialization boundary. What can be wrong with them depends on the
 * catalog, which the superRefine checks.
 */
export function ruleDraftSchemaFor(
    catalog: CatalogResponse | undefined
): z.ZodType<RuleDraft, RuleDraft> {
    return z
        .object({
            id: z.string().optional(),
            name: z.string().trim().min(1, 'Name is required'),
            scope: z.enum(['personal', 'org']),
            trigger: z.string().min(1, 'Trigger is required'),
            triggerConfig: z.object({ cron: z.string().optional(), form: z.string().optional() }),
            conditions: z.custom<RuleDraft['conditions']>(),
            actions: z
                .array(z.custom<RuleDraft['actions'][number]>())
                .min(1, 'At least one action is required'),
            enabled: z.boolean(),
            stopProcessing: z.boolean(),
            order: z.number(),
        })
        .superRefine((draft, ctx) => {
            if (!catalog) return
            for (const issue of catalogIssues(draft, catalog)) {
                ctx.addIssue({ code: 'custom', ...issue })
            }
        })
}
