import { and, eq } from '@tanstack/db'
import { useAuth } from '@tinycld/core/lib/auth'
import { captureException } from '@tinycld/core/lib/errors'
import { mutation, useMutation } from '@tinycld/core/lib/mutations'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useOrgLiveQuery } from '@tinycld/core/lib/use-org-live-query'
import { newRecordId } from 'pbtsdb/core'

export function useLabelMutations() {
    const [labelsCollection, assignmentsCollection] = useStore('labels', 'label_assignments')
    const { user } = useAuth()
    const userId = user.id

    // This user's assignments, so unassignLabel can resolve a (label,
    // record, collection) tuple to its row id through pbtsdb instead of a raw
    // pb.collection(...).getFirstListItem/delete.
    const { data: myAssignments = [] } = useOrgLiveQuery(
        (query, { userId: uid }) =>
            query
                .from({ label_assignments: assignmentsCollection })
                .where(({ label_assignments }) => eq(label_assignments.user, uid)),
        []
    )

    const onError = (error: unknown) => {
        captureException('labels.mutation', error)
    }

    const createLabel = useMutation({
        mutationFn: mutation(function* (data: { name: string; color: string }) {
            yield labelsCollection.insert({
                id: newRecordId(),
                user: userId,
                name: data.name,
                color: data.color,
            })
        }),
        onError,
    })

    const updateLabel = useMutation({
        mutationFn: mutation(function* ({
            id,
            ...data
        }: {
            id: string
            name: string
            color: string
        }) {
            yield labelsCollection.update(id, draft => {
                draft.name = data.name
                draft.color = data.color
            })
        }),
        onError,
    })

    const deleteLabel = useMutation({
        mutationFn: mutation(function* (labelId: string) {
            yield labelsCollection.delete(labelId)
        }),
        onError,
    })

    const assignLabel = useMutation({
        mutationFn: mutation(function* ({
            labelId,
            recordId,
            collection,
        }: {
            labelId: string
            recordId: string
            collection: string
        }) {
            yield assignmentsCollection.insert({
                id: newRecordId(),
                label: labelId,
                record_id: recordId,
                collection,
                user: userId,
            })
        }),
        onError,
    })

    const unassignLabel = useMutation({
        mutationFn: mutation(function* ({
            labelId,
            recordId,
            collection,
        }: {
            labelId: string
            recordId: string
            collection: string
        }) {
            const assignment = myAssignments.find(
                a => a.label === labelId && a.record_id === recordId && a.collection === collection
            )
            if (assignment) yield assignmentsCollection.delete(assignment.id)
        }),
        onError,
    })

    return { createLabel, updateLabel, deleteLabel, assignLabel, unassignLabel }
}

export function useAssignmentsForRecord(recordId: string, collection: string) {
    const [assignmentsCollection] = useStore('label_assignments')

    const { data: assignments } = useOrgLiveQuery(
        (query, { userId }) =>
            query
                .from({ label_assignments: assignmentsCollection })
                .where(({ label_assignments }) =>
                    and(
                        eq(label_assignments.record_id, recordId),
                        eq(label_assignments.collection, collection),
                        eq(label_assignments.user, userId)
                    )
                ),
        [recordId, collection]
    )

    return assignments ?? []
}
