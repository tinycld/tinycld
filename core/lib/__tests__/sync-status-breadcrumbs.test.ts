import { syncBreadcrumbs } from '@tinycld/core/lib/sync-status-breadcrumbs'
import type { SyncStatus } from 'pbtsdb'
import { describe, expect, it } from 'vitest'

const connected: SyncStatus = {
    realtime: { state: 'connected' },
    loads: { retrying: 0, failed: 0 },
}

describe('syncBreadcrumbs', () => {
    it('records a change of realtime state, with the attempt when reconnecting', () => {
        const reconnecting: SyncStatus = {
            ...connected,
            realtime: { state: 'reconnecting', attempt: 2, nextRetryAt: 2000, since: 1000 },
        }
        expect(syncBreadcrumbs(connected, reconnecting)).toEqual([
            { message: 'reconnecting', extra: { attempt: 2 } },
        ])
        expect(syncBreadcrumbs(reconnecting, connected)).toEqual([{ message: 'connected' }])
    })

    it('records loads starting and stopping retrying, not every count change', () => {
        const one: SyncStatus = { ...connected, loads: { retrying: 1, failingSince: 5, failed: 0 } }
        const three: SyncStatus = {
            ...connected,
            loads: { retrying: 3, failingSince: 5, failed: 0 },
        }
        expect(syncBreadcrumbs(connected, one)).toEqual([
            { message: 'loads retrying', extra: { retrying: 1 } },
        ])
        expect(syncBreadcrumbs(one, three)).toEqual([])
        expect(syncBreadcrumbs(three, connected)).toEqual([{ message: 'loads recovered' }])
    })

    it('records nothing for a new retry time in the same state', () => {
        const first: SyncStatus = {
            ...connected,
            realtime: { state: 'reconnecting', attempt: 1, nextRetryAt: 2000, since: 1000 },
        }
        const second: SyncStatus = {
            ...connected,
            realtime: { state: 'reconnecting', attempt: 2, nextRetryAt: 4000, since: 1000 },
        }
        expect(syncBreadcrumbs(first, second)).toEqual([])
    })
})
