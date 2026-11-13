import { and, eq } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { useAuth } from '@tinycld/core/lib/auth'
import { mutation, useMutation } from '@tinycld/core/lib/mutations'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { DEFAULT_LABEL_COLOR } from '@tinycld/core/ui/color-picker'
import { materialize } from 'pbtsdb'
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
                .select(({ t }) => ({
                    ...t,
                    labelRow: materialize(
                        query
                            .from({ l: labels })
                            .where(({ l }) => eq(l.id, t.label))
                            .findOne()
                    ),
                })),
    })
    // Stable identity: the editor resets its form whenever this changes.
    const rows = useMemo(
        () =>
            data.map(r => ({
                id: r.id,
                labelId: r.label,
                name: r.labelRow?.name ?? '',
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
            const byName = new Map(scopeLabels.map(l => [l.name.trim().toLowerCase(), l.id]))
            const keep = new Set<string>()
            for (const [order, row] of values.rows.entries()) {
                const name = row.name.trim()
                const key = name.toLowerCase()
                let labelId = byName.get(key)
                if (!labelId) {
                    labelId = newRecordId()
                    yield labels.insert({
                        id: labelId,
                        name,
                        color: DEFAULT_LABEL_COLOR,
                        user: userId,
                    })
                    byName.set(key, labelId)
                }
                const existing = rows.find(r => r.labelId === labelId)
                if (existing) {
                    keep.add(existing.id)
                    yield taxonomy.update(existing.id, d => {
                        d.assign_when = row.assignWhen.trim()
                        d.auto_action = row.autoAction
                        d.order = order
                    })
                } else {
                    yield taxonomy.insert({
                        id: newRecordId(),
                        app,
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
