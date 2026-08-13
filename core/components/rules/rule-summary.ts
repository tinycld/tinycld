// Pure, no-React: derives a one-line human summary of a rule from the
// resolved catalog. Used by RuleRow (and, indirectly, RunHistory's rule
// label) — kept out of draft.ts because it reads a plain `Rules` record, not
// a `RuleDraft`, and has no validation concerns of its own.

import type { CatalogResponse } from '@tinycld/core/lib/automation/api'
import { parseRef } from '@tinycld/core/lib/automation/helpers'
import type { Rules } from '@tinycld/core/types/pbSchema'

function conditionCount(record: Rules): number {
    const conditions = record.conditions as { groups?: { conditions?: unknown[] }[] } | null
    if (!conditions || !Array.isArray(conditions.groups)) return 0
    return conditions.groups.reduce((sum, g) => sum + (g.conditions?.length ?? 0), 0)
}

function actionRefs(record: Rules): string[] {
    const actions = record.actions as { ref?: unknown }[] | null
    if (!Array.isArray(actions)) return []
    return actions.map(a => a?.ref).filter((ref): ref is string => typeof ref === 'string')
}

/**
 * The trigger's package, iff it isn't resolvable in the catalog — either the
 * ref is malformed or the trigger simply isn't there. In both cases the
 * likely cause is an uninstalled package, so we surface its slug for a
 * "needs {pkg}" badge. Returns null when the trigger resolves fine.
 */
export function needsPackage(record: Rules, catalog: CatalogResponse): string | null {
    const trigger = catalog.triggers.find(t => t.ref === record.trigger)
    if (trigger) return null
    try {
        return parseRef(record.trigger).pkg
    } catch {
        return null
    }
}

/** "When a message arrives · 2 conditions · Move to folder, Apply label" */
export function ruleSummary(record: Rules, catalog: CatalogResponse): string {
    const trigger = catalog.triggers.find(t => t.ref === record.trigger)
    const triggerLabel = trigger?.label ?? record.trigger

    const parts: string[] = [triggerLabel]

    const count = conditionCount(record)
    if (count > 0) parts.push(`${count} condition${count === 1 ? '' : 's'}`)

    const actionsByRef = new Map(catalog.actions.map(a => [a.ref, a]))
    const actionLabels = actionRefs(record).map(ref => actionsByRef.get(ref)?.label ?? ref)
    if (actionLabels.length > 0) parts.push(actionLabels.join(', '))

    return parts.join(' · ')
}
