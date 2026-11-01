import { and, eq } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { useAuth } from '@tinycld/core/lib/auth'
import { log } from '@tinycld/core/lib/logger'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { recoverLiveQueries } from '@tinycld/core/lib/recover-live-queries'
import { useCurrentRole } from '@tinycld/core/lib/use-current-role'
import { useMyLiveQuery } from '@tinycld/core/lib/use-my-live-query'
import { useMemo } from 'react'
import { NO_ACCESS, reportAccess } from '../lib/access'
import { parseBroken, parseSpec } from '../lib/spec'

export type RecordState = 'loading' | 'error' | 'missing' | 'ready'

function recordState(input: {
    isError: boolean
    isLoading: boolean
    hasRecord: boolean
}): RecordState {
    if (input.isError) return 'error'
    if (input.isLoading) return 'loading'
    return input.hasRecord ? 'ready' : 'missing'
}

// One saved report, live, with what the reader may do with it. The record leaves
// the store when the reader loses access (the server sends a delete to the
// subscription), so a removed share shows at once.
export function useReportRecord(reportId: string) {
    const { user } = useAuth()
    const [reportsCollection, sharesCollection] = useStore('reports', 'report_shares')
    const role = useCurrentRole()
    const recordQuery = useLiveQuery({
        queryKey: ['reports.record', reportId],
        query: q =>
            q
                .from({ report: reportsCollection })
                .where(({ report }) => eq(report.id, reportId))
                .findOne(),
    })
    const sharesQuery = useMyLiveQuery(
        (q, { userId }) =>
            q
                .from({ share: sharesCollection })
                .where(({ share }) => and(eq(share.report, reportId), eq(share.user, userId))),
        { queryKey: ['reports.my-shares', reportId] }
    )
    const record = recordQuery.data
    const roles = (sharesQuery.data ?? []).map(share => share.role)
    const access = record
        ? reportAccess({
              owner: record.owner,
              published: record.published,
              shippedKey: record.shipped_key,
              userId: user.id,
              roles,
              isAdmin: role.isAdmin,
          })
        : NO_ACCESS
    const state = recordState({
        isError: recordQuery.isError || sharesQuery.isError,
        isLoading: !recordQuery.isReady || !sharesQuery.isReady || !role.isReady,
        hasRecord: record !== undefined,
    })
    const retry = () => {
        void recoverLiveQueries({
            queries: [recordQuery.collection, sharesQuery.collection],
            sources: [reportsCollection, sharesCollection],
            onError: error => log.error('reports.record', error),
        })
    }
    // Parsed once per stored value, so the builder's defaults keep their identity
    // across renders.
    const storedSpec = record?.spec
    const storedBroken = record?.broken
    const spec = useMemo(() => parseSpec(storedSpec), [storedSpec])
    const broken = useMemo(() => parseBroken(storedBroken), [storedBroken])
    return {
        record,
        spec,
        broken,
        access,
        state,
        retry,
    }
}

export type SavedReport = NonNullable<ReturnType<typeof useReportRecord>['record']>
