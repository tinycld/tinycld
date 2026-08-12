import { z } from 'zod'
import { ALL_OPS, NO_VALUE_OPS } from './helpers'
import type { AutomationDefinitions, ConditionOp } from './types'

const ID_RE = /^[a-z0-9]+(-[a-z0-9]+)*$/
const REF_RE = /^[a-z0-9-]+:[a-z0-9-]+$/

export const conditionSchema = z
    .object({
        field: z.string().min(1),
        op: z.enum(ALL_OPS as [ConditionOp, ...ConditionOp[]]),
        value: z.union([z.string(), z.number(), z.boolean()]).optional(),
    })
    .superRefine((c, ctx) => {
        if (!NO_VALUE_OPS.has(c.op) && c.value === undefined) {
            ctx.addIssue({ code: 'custom', message: `operator '${c.op}' requires a value` })
        }
        if (NO_VALUE_OPS.has(c.op) && c.value !== undefined) {
            ctx.addIssue({ code: 'custom', message: `operator '${c.op}' takes no value` })
        }
    })

export const conditionGroupSchema = z.object({
    match: z.enum(['all', 'any']),
    conditions: z.array(conditionSchema).min(1),
})

// One level of grouping by construction: groups contain conditions, never
// other groups (spec: condition AST).
export const conditionsAstSchema = z.object({
    match: z.enum(['all', 'any']),
    groups: z.array(conditionGroupSchema),
})

export const ruleActionSchema = z.object({
    ref: z.string().regex(REF_RE, 'action ref must be qualified as <pkg>:<id>'),
    params: z.record(z.string(), z.union([z.string(), z.number(), z.boolean()])).default({}),
})

export const ruleActionsSchema = z.array(ruleActionSchema).min(1)

function checkId(errors: string[], pkgSlug: string, what: string, id: string, seen: Set<string>) {
    if (!ID_RE.test(id)) {
        errors.push(`${pkgSlug}: ${what} id '${id}' must be kebab-case ([a-z0-9-])`)
    }
    if (seen.has(id)) errors.push(`${pkgSlug}: duplicate ${what} id '${id}'`)
    seen.add(id)
}

/**
 * Structural validation of a package's automation definitions. Collection and
 * column EXISTENCE is not checked here — the package's own typecheck enforces
 * it at compile time, and Phase 2's Go resolution re-checks at runtime.
 */
export function validateDefinitions(
    pkgSlug: string,
    defs: AutomationDefinitions,
    opts: { allowSynthetic?: boolean } = {}
): string[] {
    const errors: string[] = []
    const triggerIds = new Set<string>()
    for (const t of defs.triggers ?? []) {
        checkId(errors, pkgSlug, 'trigger', t.id, triggerIds)
        if ('synthetic' in t) {
            if (!opts.allowSynthetic) {
                errors.push(
                    `${pkgSlug}: trigger '${t.id}' is synthetic — only core may declare synthetic triggers`
                )
            }
        } else {
            if (!t.collection) {
                errors.push(`${pkgSlug}: trigger '${t.id}' has no collection`)
            }
            if (t.watch && t.on !== 'update') {
                errors.push(
                    `${pkgSlug}: trigger '${t.id}' declares watch but is not an update trigger`
                )
            }
            if (t.fields && t.fields.length === 0) {
                errors.push(
                    `${pkgSlug}: trigger '${t.id}' has an empty fields list — omit fields to expose all columns`
                )
            }
        }
    }
    const actionIds = new Set<string>()
    for (const a of defs.actions ?? []) {
        checkId(errors, pkgSlug, 'action', a.id, actionIds)
        const paramKeys = new Set<string>()
        for (const p of a.params ?? []) {
            if (paramKeys.has(p.key))
                errors.push(`${pkgSlug}: action '${a.id}' duplicate param '${p.key}'`)
            paramKeys.add(p.key)
        }
        if (a.kind === 'record-op') {
            if (!a.collection)
                errors.push(`${pkgSlug}: record-op action '${a.id}' has no collection`)
            if (a.op.type !== 'delete') {
                for (const v of Object.values(a.op.set)) {
                    if (
                        typeof v === 'object' &&
                        v !== null &&
                        'param' in v &&
                        !paramKeys.has(v.param)
                    ) {
                        errors.push(
                            `${pkgSlug}: action '${a.id}' set references undeclared param '${v.param}'`
                        )
                    }
                }
            }
        }
    }
    return errors
}
