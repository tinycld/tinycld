// @vitest-environment happy-dom
import { createCollection, localOnlyCollectionOptions } from '@tanstack/db'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, renderHook, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

// The hook runs on every app load for owners and admins, so it must read only
// the wizard row, never the whole system_settings collection (which holds
// secrets such as the VAPID private key).

type Row = { id: string; key: string; value: string; is_secret: boolean }

const wizardValue = JSON.stringify({ startedAt: 'x', acknowledged: [], skipped: [] })

let collectionCount = 0

function settingsOf(rows: Row[]) {
    collectionCount += 1
    return createCollection(
        localOnlyCollectionOptions({
            id: `system-settings-${collectionCount}`,
            getKey: (r: Row) => r.id,
            initialData: rows,
        })
    )
}

const h = vi.hoisted(() => ({ settings: null as unknown }))

vi.mock('@tinycld/core/lib/pocketbase', () => ({
    useStore: () => [h.settings],
}))

import { useSetupWizardState } from '../use-setup-wizard-state'

function wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={new QueryClient()}>{children}</QueryClientProvider>
}

afterEach(cleanup)

describe('useSetupWizardState', () => {
    it('reads the wizard row among other settings', async () => {
        h.settings = settingsOf([
            { id: 's1', key: 'vapid.private_key', value: 'secret', is_secret: true },
            { id: 's2', key: 'setup.wizard', value: wizardValue, is_secret: false },
        ])
        const { result } = renderHook(() => useSetupWizardState(), { wrapper })
        await waitFor(() => expect(result.current.isReady).toBe(true))
        expect(result.current.state).toEqual({ startedAt: 'x', acknowledged: [], skipped: [] })
    })

    it('writes a patch to the wizard row', async () => {
        h.settings = settingsOf([
            { id: 's2', key: 'setup.wizard', value: wizardValue, is_secret: false },
        ])
        const { result } = renderHook(() => useSetupWizardState(), { wrapper })
        await waitFor(() => expect(result.current.state).not.toBeNull())
        await act(() => result.current.update(s => ({ ...s, acknowledged: ['core:workspace'] })))
        await waitFor(() => expect(result.current.state?.acknowledged).toEqual(['core:workspace']))
    })

    it('is null when no wizard row exists', async () => {
        h.settings = settingsOf([{ id: 's1', key: 'sentry.dsn', value: '', is_secret: false }])
        const { result } = renderHook(() => useSetupWizardState(), { wrapper })
        await waitFor(() => expect(result.current.isReady).toBe(true))
        expect(result.current.state).toBeNull()
    })
})
