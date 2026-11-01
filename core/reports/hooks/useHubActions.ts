import { useAuth } from '@tinycld/core/lib/auth'
import { errorToString } from '@tinycld/core/lib/errors'
import { useMutation } from '@tinycld/core/lib/mutations'
import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useToastStore } from '@tinycld/core/lib/stores/toast-store'
import { router } from 'expo-router'
import { newRecordId } from 'pbtsdb'
import { copyTitle } from './useReportBuilder'
import type { HubReport } from './useReportsHub'

/** The parts of a saved report (as the reports queries read it) an action needs. */
export type ActionReport = Pick<HubReport, 'id' | 'title' | 'description' | 'spec'>

function toastError(title: string, error: unknown) {
    useToastStore
        .getState()
        .addToast({ title, body: errorToString(error), variant: 'error', duration: 6000 })
}

// The hub's and the report's actions on a saved report. Duplicate and Save as are a
// plain create with the stored spec (the sharing contracts); the server refuses what
// the reader may not do, and says why in a toast.
export function useHubActions() {
    const { user } = useAuth()
    const orgHref = useOrgHref()
    const [reportsCollection] = useStore('reports')
    const copy = useMutation({
        mutationFn: function* ({ report, title }: { report: ActionReport; title: string }) {
            const id = newRecordId()
            yield reportsCollection.insert({
                id,
                owner: user.id,
                title,
                description: report.description,
                spec: report.spec,
                published: false,
            })
            return id
        },
        // A push, so Back returns to the report the copy was made from.
        onSuccess: id => router.push(orgHref('reports/[id]', { id })),
        onError: error => toastError('Could not copy the report', error),
    })
    const remove = useMutation({
        mutationFn: function* (report: ActionReport) {
            yield reportsCollection.delete(report.id)
        },
        onError: error => toastError('Could not delete the report', error),
    })
    const publish = useMutation({
        mutationFn: function* ({
            report,
            published,
        }: {
            report: ActionReport
            published: boolean
        }) {
            yield reportsCollection.update(report.id, row => {
                row.published = published
            })
        },
        onError: error => toastError('Could not change who sees the report', error),
    })
    return {
        duplicate: (report: ActionReport) =>
            copy.mutate({ report, title: copyTitle(report.title) }),
        saveCopy: (report: ActionReport, title: string) => copy.mutate({ report, title }),
        remove: (report: ActionReport) => remove.mutate(report),
        setPublished: (report: ActionReport, published: boolean) =>
            publish.mutate({ report, published }),
        isPending: copy.isPending || remove.isPending || publish.isPending,
    }
}

export type HubActions = ReturnType<typeof useHubActions>
