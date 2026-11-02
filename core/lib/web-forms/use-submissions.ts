import { eq, inArray } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { useMutation } from '@tinycld/core/lib/mutations'
import { pb, useStore } from '@tinycld/core/lib/pocketbase'
import { useMemo } from 'react'

/** The newest `limit` submissions of a form, newest first. */
export function useSubmissions(formId: string, limit: number) {
    const [submissionsCollection] = useStore('form_submissions')
    const { data, isReady } = useLiveQuery({
        query: query =>
            query
                .from({ s: submissionsCollection })
                .where(({ s }) => eq(s.form, formId))
                .orderBy(({ s }) => s.created, 'desc')
                .limit(limit),
    })
    const submissions = data ?? []
    return { submissions, hasMore: submissions.length >= limit, isReady: Boolean(isReady) }
}

export type SubmissionRow = ReturnType<typeof useSubmissions>['submissions'][number]

/** The rule runs for one submission, newest first. */
export function useSubmissionRuns(submissionId: string) {
    const [runsCollection] = useStore('rule_runs')
    const { data } = useLiveQuery({
        query: query =>
            query
                .from({ r: runsCollection })
                .where(({ r }) => eq(r.record_id, submissionId))
                .orderBy(({ r }) => r.fired_at, 'desc'),
    })
    return data ?? []
}

/**
 * The rule runs of a page of submissions, by submission id, in one query: a
 * query for each row would open a subscription for each row.
 */
export function useRunsBySubmission(submissionIds: string[]) {
    const [runsCollection] = useStore('rule_runs')
    const { data } = useLiveQuery({
        query: query => {
            // inArray over no ids is not a valid filter; there is nothing to read.
            if (submissionIds.length === 0) return null
            return query
                .from({ r: runsCollection })
                .where(({ r }) => inArray(r.record_id, submissionIds))
        },
    })
    return useMemo(() => {
        const bySubmission = new Map<string, NonNullable<typeof data>>()
        for (const run of data ?? []) {
            const list = bySubmission.get(run.record_id) ?? []
            list.push(run)
            bySubmission.set(run.record_id, list)
        }
        return bySubmission
    }, [data])
}

export function useSubmissionFiles(submissionId: string) {
    const [filesCollection] = useStore('form_submission_files')
    const { data } = useLiveQuery({
        query: query =>
            query.from({ f: filesCollection }).where(({ f }) => eq(f.submission, submissionId)),
    })
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
