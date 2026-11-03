import { and, eq, inArray } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { FORM_SUBMITTED_TRIGGER } from '@tinycld/core/lib/automation/form-trigger'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useMemo } from 'react'

function formOf(config: unknown): string | undefined {
    if (!config || typeof config !== 'object') return undefined
    const form = (config as { form?: unknown }).form
    return typeof form === 'string' ? form : undefined
}

/**
 * The org rules that deliver this form's submissions, each one's last run,
 * and the order a new destination takes (after every web form rule, so no
 * two run in an undefined order).
 */
export function useDestinations(formId: string) {
    const [rulesCollection, runsCollection] = useStore('rules', 'rule_runs')
    const { data, isReady } = useLiveQuery({
        query: query =>
            query
                .from({ r: rulesCollection })
                .where(({ r }) => and(eq(r.scope, 'org'), eq(r.trigger, FORM_SUBMITTED_TRIGGER)))
                .orderBy(({ r }) => r.order, 'asc'),
    })
    // trigger_config is json, which a where cannot reach into; filter after.
    const rules = useMemo(
        () => (data ?? []).filter(r => formOf(r.trigger_config) === formId),
        [data, formId]
    )
    const ruleIds = useMemo(() => rules.map(r => r.id), [rules])
    const nextOrder = useMemo(() => Math.max(-1, ...(data ?? []).map(r => r.order)) + 1, [data])

    const { data: runs } = useLiveQuery({
        query: query => {
            // inArray over no ids is not a valid filter, and there is nothing to read.
            if (ruleIds.length === 0) return null
            return query
                .from({ run: runsCollection })
                .where(({ run }) => inArray(run.rule, ruleIds))
        },
    })
    const lastRunByRule = useMemo(() => {
        const latest = new Map<string, { fired_at: string; matched: boolean }>()
        for (const run of runs ?? []) {
            const existing = latest.get(run.rule)
            if (!existing || run.fired_at > existing.fired_at) {
                latest.set(run.rule, { fired_at: run.fired_at, matched: run.matched })
            }
        }
        return latest
    }, [runs])

    return { rules, lastRunByRule, nextOrder, isReady: Boolean(isReady) }
}
