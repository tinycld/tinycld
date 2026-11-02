import { eq } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { useMutation } from '@tinycld/core/lib/mutations'
import { pb, useStore } from '@tinycld/core/lib/pocketbase'

/** A form's submissions, newest first. */
export function useSubmissions(formId: string) {
    const [submissionsCollection] = useStore('form_submissions')
    const { data, isReady } = useLiveQuery(
        query =>
            query
                .from({ s: submissionsCollection })
                .where(({ s }) => eq(s.form, formId))
                .orderBy(({ s }) => s.created, 'desc'),
        [submissionsCollection, formId]
    )
    return { submissions: data ?? [], isReady: Boolean(isReady) }
}

export type SubmissionRow = ReturnType<typeof useSubmissions>['submissions'][number]

/** The rule runs for one submission, newest first. */
export function useSubmissionRuns(submissionId: string) {
    const [runsCollection] = useStore('rule_runs')
    const { data } = useLiveQuery(
        query =>
            query
                .from({ r: runsCollection })
                .where(({ r }) => eq(r.record_id, submissionId))
                .orderBy(({ r }) => r.fired_at, 'desc'),
        [runsCollection, submissionId]
    )
    return data ?? []
}

export function useSubmissionFiles(submissionId: string) {
    const [filesCollection] = useStore('form_submission_files')
    const { data } = useLiveQuery(
        query =>
            query.from({ f: filesCollection }).where(({ f }) => eq(f.submission, submissionId)),
        [filesCollection, submissionId]
    )
    return data ?? []
}

/** Runs one destination again for one submission; the new run arrives through rule_runs. */
export function useRetry() {
    return useMutation({
        mutationFn: async ({ ruleId, submissionId }: { ruleId: string; submissionId: string }) =>
            await pb.send(`/api/automation/rules/${ruleId}/records/${submissionId}/run`, {
                method: 'POST',
            }),
    })
}

export function useDeleteSubmission() {
    const [submissionsCollection] = useStore('form_submissions')
    return useMutation({
        mutationFn: function* (id: string) {
            yield submissionsCollection.delete(id)
        },
    })
}
