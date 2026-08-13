import { useAuth } from '@tinycld/core/lib/auth'
import { mutation, useMutation } from '@tinycld/core/lib/mutations'
import { pb, useStore } from '@tinycld/core/lib/pocketbase'
import { newRecordId } from 'pbtsdb/core'
import type { DryRunRequest, DryRunResponse, RunResponse } from './api'
import type { RuleDraft } from './draft'
import { draftToRecord } from './draft'

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
                yield rulesCollection.insert({ id: newRecordId(), owner: userId, ...fields })
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
    const dryRun = useMutation({
        mutationFn: async (body: DryRunRequest): Promise<DryRunResponse> =>
            await pb.send('/api/automation/dry-run', { method: 'POST', body }),
    })

    return { save, remove, setEnabled, reorder, runNow, dryRun }
}
