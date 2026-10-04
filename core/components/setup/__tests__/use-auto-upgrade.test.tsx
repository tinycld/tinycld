// @vitest-environment happy-dom

// A failed write of the on/off switch must not revert silently: both the
// Packages page and the setup wizard use this hook, so the hook itself reports
// the failure. The store and the live query are the data-layer boundary here
// (same posture as backups-section.test.tsx); the mutation machinery, the
// notify pipeline and the toast store are the real ones.

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { renderHook, waitFor } from '@testing-library/react'
import { useToastStore } from '@tinycld/core/lib/stores/toast-store'
import type PocketBase from 'pocketbase'
import type { ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const update = vi.fn((_id: string, _fn: unknown) => {
    throw new Error('rule denied the write')
})

vi.mock('@tinycld/core/lib/pocketbase', () => ({
    useStore: () => [{ update, insert: vi.fn() }],
    notificationsCollection: {
        insert: vi.fn(() => ({ isPersisted: { promise: Promise.resolve() } })),
    },
}))
vi.mock('@tanstack/react-db', () => ({
    useLiveQuery: () => ({
        data: [{ id: 'row1', key: 'autoupgrade.enabled', value: 'true', is_secret: false }],
        isReady: true,
    }),
}))
vi.mock('@tinycld/core/lib/notifications', () => ({
    showOsNotification: vi.fn(() => Promise.resolve()),
}))
vi.mock('@tinycld/core/lib/sentry', () => ({
    captureExceptionToSentry: vi.fn(),
    addBreadcrumbToSentry: vi.fn(),
}))

import { useAutoUpgrade } from '../use-auto-upgrade'

function wrapper() {
    const client = new QueryClient({ defaultOptions: { mutations: { retry: false } } })
    return ({ children }: { children: ReactNode }) => (
        <QueryClientProvider client={client}>{children}</QueryClientProvider>
    )
}

describe('useAutoUpgrade.setOn', () => {
    beforeEach(() => {
        useToastStore.setState({ toasts: [] })
        update.mockClear()
    })

    it('shows an error when the write fails', async () => {
        const { result } = renderHook(() => useAutoUpgrade({} as PocketBase, false), {
            wrapper: wrapper(),
        })
        expect(result.current.isOn).toBe(true)

        result.current.setOn(false)

        await waitFor(() => expect(useToastStore.getState().toasts).toHaveLength(1))
        expect(update).toHaveBeenCalledWith('row1', expect.any(Function))
        const toast = useToastStore.getState().toasts[0]
        expect(toast).toMatchObject({
            title: 'Could not change automatic updates',
            variant: 'error',
        })
        expect(toast.body).toContain('rule denied the write')
    })
})
