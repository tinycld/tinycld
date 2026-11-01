import { and, eq, ilike } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { errorToString } from '@tinycld/core/lib/errors'
import { useGroupGrants } from '@tinycld/core/lib/groups/use-group-grants'
import { useMutation } from '@tinycld/core/lib/mutations'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useToastStore } from '@tinycld/core/lib/stores/toast-store'
import { newRecordId } from 'pbtsdb'
import { useState } from 'react'
import { SHARE_ROLES, type ShareRole } from '../lib/access'
import { useReportRecord } from './useReportRecord'

const CANDIDATE_LIMIT = 20

function toastError(error: unknown) {
    useToastStore.getState().addToast({
        title: 'Could not change sharing',
        body: errorToString(error),
        variant: 'error',
        duration: 6000,
    })
}

// People the report can be shared with, searched on the server by name prefix. A
// guest may receive a share (the sharing contracts), so roles are not filtered.
function useShareCandidates(search: string, exclude: ReadonlySet<string>) {
    const [usersCollection] = useStore('users')
    // pbtsdb escapes `"` but not `\`, so a backslash would break PocketBase's filter
    // literal; it means nothing in a name search.
    const term = search.trim().replace(/\\/g, '')
    const { data } = useLiveQuery({
        queryKey: ['reports.share-candidates', term],
        query: q =>
            term
                ? q
                      .from({ u: usersCollection })
                      // ilike: PocketBase's `~` ignores case, and so must the store's
                      // own match, or a lower-case search drops the rows it loaded.
                      .where(({ u }) => and(eq(u.disabled, false), ilike(u.name, `${term}%`)))
                      .orderBy(({ u }) => u.name)
                      .limit(CANDIDATE_LIMIT)
                      .select(({ u }) => ({ id: u.id, name: u.name }))
                : null,
    })
    return (data ?? []).filter(candidate => !exclude.has(candidate.id))
}

// The share dialog's data: direct shares (a user, no group), group grants through
// core's group share section, the people search and the publish switch. Each
// change is written at once; a refusal is a toast.
export function useReportShares(reportId: string) {
    const record = useReportRecord(reportId)
    const [sharesCollection, reportsCollection] = useStore('report_shares', 'reports')
    const { data: direct } = useLiveQuery({
        queryKey: ['reports.shares.direct', reportId],
        query: q =>
            q
                .from({ share: sharesCollection })
                .where(({ share }) => and(eq(share.report, reportId), eq(share.group, ''))),
    })
    const groups = useGroupGrants({
        collection: sharesCollection,
        roles: SHARE_ROLES,
        isForResource: row => row.report === reportId,
        buildRow: grant => ({ ...grant, report: reportId }),
    })
    // The search box's text and the role a pick adds: local UI state.
    const [search, setSearch] = useState('')
    const [role, setRole] = useState<ShareRole>('viewer')
    const directRows = direct ?? []
    const owner = record.record?.owner ?? ''
    const exclude = new Set([owner, ...directRows.map(row => row.user)])
    const candidates = useShareCandidates(search, exclude)
    const add = useMutation({
        mutationFn: function* (userId: string) {
            yield sharesCollection.insert({
                id: newRecordId(),
                report: reportId,
                user: userId,
                group: '',
                role,
            })
        },
        onSuccess: () => setSearch(''),
        onError: toastError,
    })
    const changeRole = useMutation({
        mutationFn: function* ({ shareId, next }: { shareId: string; next: ShareRole }) {
            yield sharesCollection.update(shareId, row => {
                row.role = next
            })
        },
        onError: toastError,
    })
    const remove = useMutation({
        mutationFn: function* (shareId: string) {
            yield sharesCollection.delete(shareId)
        },
        onError: toastError,
    })
    const publish = useMutation({
        mutationFn: function* (published: boolean) {
            yield reportsCollection.update(reportId, row => {
                row.published = published
            })
        },
        onError: toastError,
    })
    return {
        title: record.record?.title ?? '',
        owner,
        published: record.record?.published === true,
        access: record.access,
        direct: directRows,
        groups,
        candidates,
        search,
        setSearch,
        role,
        setRole,
        add: (userId: string) => add.mutate(userId),
        changeRole: (shareId: string, next: ShareRole) => changeRole.mutate({ shareId, next }),
        remove: (shareId: string) => remove.mutate(shareId),
        setPublished: (published: boolean) => publish.mutate(published),
    }
}

export type ReportShares = ReturnType<typeof useReportShares>
