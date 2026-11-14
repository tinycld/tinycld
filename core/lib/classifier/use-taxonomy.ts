import { and, eq } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { useAuth } from '@tinycld/core/lib/auth'
import { mutation, useMutation } from '@tinycld/core/lib/mutations'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { DEFAULT_LABEL_COLOR } from '@tinycld/core/ui/color-picker'
import { newRecordId } from 'pbtsdb/core'
import { useMemo } from 'react'
import type { TaxonomyFormValues } from './taxonomy-schema'

export type TaxonomyScope = 'org' | 'personal'

export function useTaxonomy(app: string, scope: TaxonomyScope) {
    const { user } = useAuth()
    // Org rows and labels store an empty user, personal ones the owner's id.
    const userId = scope === 'personal' ? user.id : ''
    const [taxonomy, labels] = useStore('classifier_labels', 'labels')

    const { data = [], isReady } = useLiveQuery({
        queryKey: ['taxonomy', app, userId],
        query: query =>
            query
                .from({ t: taxonomy })
                .where(({ t }) => and(eq(t.app, app), eq(t.user, userId)))
                .orderBy(({ t }) => t.order)
                .select(({ t }) => ({ ...t })),
    })
    // Stable identity: the editor resets its form whenever this changes.
    const rows = useMemo(
        () =>
            data.map(r => ({
                id: r.id,
                labelId: r.label,
                name: r.name,
                assignWhen: r.assign_when,
                autoAction: r.auto_action,
            })),
        [data]
    )

    // A label already in this scope is reused by name, so re-saving or
    // re-adding a name never trips the unique (user, name) rule.
    const { data: scopeLabels = [] } = useLiveQuery({
        queryKey: ['taxonomy-labels', userId],
        query: query => query.from({ l: labels }).where(({ l }) => eq(l.user, userId)),
    })

    const save = useMutation({
        mutationFn: mutation(function* (values: TaxonomyFormValues) {
            const byName = new Map(scopeLabels.map(l => [l.name.trim().toLowerCase(), l]))
            // A label another taxonomy row already points at cannot back a new
            // row: the (app, user, label) index allows one row per label.
            const usedLabelIds = new Set(rows.map(r => r.labelId))
            const keep = new Set<string>()
            for (const [order, row] of values.rows.entries()) {
                const name = row.name.trim()
                const key = name.toLowerCase()
                const existing = rows.find(r => r.name.trim().toLowerCase() === key)
                if (existing) {
                    // The label stays: a member may have renamed it since, and
                    // re-resolving by name could move the row to a different label.
                    keep.add(existing.id)
                    yield taxonomy.update(existing.id, d => {
                        d.name = name
                        d.assign_when = row.assignWhen.trim()
                        d.auto_action = row.autoAction
                        d.order = order
                    })
                } else {
                    const found = byName.get(key)
                    const reusable = found && !usedLabelIds.has(found.id) ? found : undefined
                    const labelId = reusable?.id ?? newRecordId()
                    if (!reusable) {
                        yield labels.insert({
                            id: labelId,
                            name,
                            color: DEFAULT_LABEL_COLOR,
                            user: userId,
                        })
                    }
                    usedLabelIds.add(labelId)
                    yield taxonomy.insert({
                        id: newRecordId(),
                        app,
                        // The reused label's spelling, so the chip and the badge match.
                        name: reusable?.name ?? name,
                        label: labelId,
                        assign_when: row.assignWhen.trim(),
                        auto_action: row.autoAction,
                        order,
                        user: userId,
                    })
                }
            }
            for (const r of rows) {
                if (!keep.has(r.id)) yield taxonomy.delete(r.id)
            }
        }),
    })
    return { rows, isReady, save }
}
