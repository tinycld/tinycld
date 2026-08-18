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

// `useCurrentUserId`/`useOrgLiveQuery` doesn't export a bare user-id hook —
// the established idiom (see useLabelMutations) is `useAuth().user.id`.
export function useRuleMutations() {
    const [rulesCollection] = useStore('rules')
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
                yield rulesCollection.insert({
                    id: newRecordId(),
                    owner: userId,
                    ...fields,
                    order: nextOrderFor(rulesCollection.toArray),
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
