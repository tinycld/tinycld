import { useLiveQuery } from '@tanstack/react-db'
import { useAuth } from '@tinycld/core/lib/auth'
import { mutation, useMutation } from '@tinycld/core/lib/mutations'
import { pb, useStore } from '@tinycld/core/lib/pocketbase'
import { newRecordId } from 'pbtsdb/core'
import type { DryRunRequest, DryRunResponse, DryRunResult, RunResponse } from './api'
import type { RuleDraft } from './draft'
import { draftToRecord } from './draft'

// A new rule sorts after every existing one. Ties (several rules sharing an
// order) make the displayed sequence and the execution sequence disagree, so
// this is read at insert time from the live collection.
function nextOrderFor(rules: { order: number }[]): number {
    if (rules.length === 0) return 0
    return Math.max(...rules.map(r => r.order)) + 1
}

// `order` is a GLOBAL column, not per-scope: the engine sorts a trigger's rules
// by `order` across both scopes (automation/engine.go FindRecordsByFilter sorts
// on it, and its subsequent SliceStable only lifts org above personal as a
// stable tier — it never re-numbers within one). So the max must be taken over
// every rule, and `rules` is on-demand: its store holds only the scope
// RulesPanel happens to have loaded. This live query asks for all of them, so
// the store has both scopes before the max is computed.
function useAllRuleOrders() {
    const [rulesCollection] = useStore('rules')
    const { data: rows } = useLiveQuery({
        query: query => query.from({ r: rulesCollection }).select(({ r }) => ({ order: r.order })),
    })
    return rows ?? []
}

// `useCurrentUserId`/`useMyLiveQuery` doesn't export a bare user-id hook —
// the established idiom (see useLabelMutations) is `useAuth().user.id`.
export function useRuleMutations() {
    const [rulesCollection] = useStore('rules')
    const allOrders = useAllRuleOrders()
    const { user } = useAuth()
    const userId = user.id

    const save = useMutation({
        mutationFn: mutation(function* (draft: RuleDraft) {
            const fields = draftToRecord(draft)
            if (draft.id) {
                yield rulesCollection.update(draft.id, r => Object.assign(r, fields))
            } else {
                // Order is computed HERE, not captured when the builder opened:
                // two tabs (or a builder left open while rules changed) would
                // otherwise both seed the same stale "max + 1" and land on the
                // same order, where ties make display and execution diverge.
                // `allOrders` is a live query, so it re-renders this hook on
                // every rule change and the value read here is the current one.
                yield rulesCollection.insert({
                    id: newRecordId(),
                    owner: userId,
                    ...fields,
                    order: nextOrderFor(allOrders),
                })
            }
        }),
    })
    const remove = useMutation({
        mutationFn: mutation(function* (id: string) {
            yield rulesCollection.delete(id)
        }),
    })
    const setEnabled = useMutation({
        mutationFn: mutation(function* ({ id, enabled }: { id: string; enabled: boolean }) {
            yield rulesCollection.update(id, r => {
                r.enabled = enabled
            })
        }),
    })
    // First order-column reindex in the codebase: one parallel array-yield,
    // renumbering every row to its new index keeps the invariant simple.
    const reorder = useMutation({
        mutationFn: mutation(function* (orderedIds: string[]) {
            yield orderedIds.map((id, index) =>
                rulesCollection.update(id, r => {
                    r.order = index
                })
            )
        }),
    })
    const runNow = useMutation({
        mutationFn: async (id: string): Promise<RunResponse> =>
            await pb.send(`/api/automation/rules/${id}/run`, { method: 'POST' }),
    })
    // Normalized here so no consumer has to: `matches` is nullable on the wire
    // (Go marshals an uninitialized slice as null), and the panel maps over it.
    const dryRun = useMutation({
        mutationFn: async (body: DryRunRequest): Promise<DryRunResult> => {
            const res: DryRunResponse = await pb.send('/api/automation/dry-run', {
                method: 'POST',
                body,
            })
            return { total: res.total, matches: res.matches ?? [] }
        },
    })

    return { save, remove, setEnabled, reorder, runNow, dryRun }
}
