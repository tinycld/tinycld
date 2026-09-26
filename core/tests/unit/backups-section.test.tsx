// @vitest-environment happy-dom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

// The section only presents what useBackupRows hands it, so the query is
// mocked and the collections never resolve. Same posture as
// connected-apps.test.tsx.
const rowsMock = vi.fn<() => { data: unknown[] | undefined }>(() => ({ data: [] }))
vi.mock('../../components/settings/backups/useBackups', async importOriginal => {
    const actual =
        await importOriginal<typeof import('../../components/settings/backups/useBackups')>()
    return { ...actual, useBackupRows: () => rowsMock() }
})
vi.mock('@tinycld/core/lib/use-current-role', () => ({
    useCurrentRole: () => ({ isOwner: true, isAdmin: true, isReady: true }),
}))
vi.mock('@tinycld/core/lib/pocketbase', () => ({
    useStore: () => [{}, {}],
    pb: { send: vi.fn() },
}))

import { BackupsSection } from '../../components/settings/backups/BackupsSection'
import { isAwaitingRestart, lastBackedUp } from '../../components/settings/backups/useBackups'

afterEach(() => {
    cleanup()
    rowsMock.mockReset()
    rowsMock.mockReturnValue({ data: [] })
})

function renderSection() {
    return render(
        <QueryClientProvider client={new QueryClient()}>
            <BackupsSection />
        </QueryClientProvider>
    )
}

describe('BackupsSection', () => {
    it('says never backed up with an empty ledger', () => {
        renderSection()
        expect(screen.getByText('Never backed up')).toBeTruthy()
    })

    it('shows the newest succeeded run', () => {
        const recent = new Date(Date.now() - 3 * 3600 * 1000).toISOString()
        rowsMock.mockReturnValue({
            data: [
                {
                    id: 'b1',
                    kind: 'manual',
                    status: 'succeeded',
                    started: recent,
                    finished: recent,
                    bytes: 2048,
                    error: '',
                    metadata: null,
                    initiatorName: 'Ada',
                },
            ],
        })
        renderSection()
        expect(screen.getByText('Last backed up 3h ago')).toBeTruthy()
        expect(screen.getByText('Succeeded')).toBeTruthy()
    })

    it('labels a job that is waiting for a fresh source', () => {
        const recent = new Date(Date.now() - 60 * 1000).toISOString()
        rowsMock.mockReturnValue({
            data: [
                {
                    id: 'r1',
                    kind: 'restore',
                    status: 'waiting_for_source',
                    started: recent,
                    finished: '',
                    bytes: 0,
                    error: '',
                    metadata: null,
                    initiatorName: 'Ada',
                },
            ],
        })
        renderSection()
        expect(screen.getByText('Waiting for source')).toBeTruthy()
    })
})

describe('a restore awaiting a restart', () => {
    const row = (metadata: unknown) => ({
        id: 'r2',
        kind: 'restore' as const,
        status: 'running' as const,
        started: new Date().toISOString(),
        finished: '',
        bytes: 0,
        error: '',
        metadata,
        initiatorName: 'Ada',
    })

    it('tells the operator a restart is owed', () => {
        rowsMock.mockReturnValue({ data: [row({ awaiting_restart: true })] })
        renderSection()
        expect(screen.getByText('Restart the server to complete this restore.')).toBeTruthy()
    })

    it('says nothing about a restart for a restore that is simply running', () => {
        rowsMock.mockReturnValue({ data: [row(null)] })
        renderSection()
        expect(screen.queryByText('Restart the server to complete this restore.')).toBeNull()
    })

    it('is false for a finished restore that once carried the flag', () => {
        expect(
            isAwaitingRestart({
                kind: 'restore',
                status: 'succeeded',
                metadata: { awaiting_restart: true },
            } as never)
        ).toBe(false)
    })

    it('is false for a backup row', () => {
        expect(
            isAwaitingRestart({
                kind: 'manual',
                status: 'running',
                metadata: { awaiting_restart: true },
            } as never)
        ).toBe(false)
    })
})

describe('lastBackedUp', () => {
    it('flags stale after 7 days', () => {
        const old = new Date(Date.now() - 8 * 86400 * 1000).toISOString()
        const r = lastBackedUp([{ status: 'succeeded', finished: old, kind: 'manual' } as never])
        expect(r.isStale).toBe(true)
    })

    it('is not stale for a recent run', () => {
        const recent = new Date(Date.now() - 2 * 3600 * 1000).toISOString()
        const r = lastBackedUp([{ status: 'succeeded', finished: recent, kind: 'manual' } as never])
        expect(r.isStale).toBe(false)
        expect(r.label).toBe('Last backed up 2h ago')
    })

    it('picks the run that finished last, not the one that started first', () => {
        const startedFirst = new Date(Date.now() - 9 * 3600 * 1000).toISOString()
        // The rows arrive in `started` order, newest first, so the long run that
        // started earlier is LAST in the list — and is the one that finished most
        // recently.
        const r = lastBackedUp([
            { status: 'succeeded', finished: startedFirst, kind: 'manual' } as never,
            {
                status: 'succeeded',
                finished: new Date(Date.now() - 2 * 3600 * 1000).toISOString(),
                kind: 'scheduled',
            } as never,
        ])
        expect(r.label).toBe('Last backed up 2h ago')
        expect(r.isStale).toBe(false)
    })

    it('skips a succeeded row with no finish time', () => {
        const recent = new Date(Date.now() - 2 * 3600 * 1000).toISOString()
        const r = lastBackedUp([
            { status: 'succeeded', finished: '', kind: 'manual' } as never,
            { status: 'succeeded', finished: recent, kind: 'manual' } as never,
        ])
        expect(r.label).toBe('Last backed up 2h ago')
    })

    it('ignores a restore row when reporting the last backup', () => {
        const recent = new Date(Date.now() - 2 * 3600 * 1000).toISOString()
        const r = lastBackedUp([
            { status: 'succeeded', finished: recent, kind: 'restore' } as never,
        ])
        expect(r.label).toBe('Never backed up')
        expect(r.isStale).toBe(true)
    })
})
