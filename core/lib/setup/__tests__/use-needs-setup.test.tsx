// @vitest-environment happy-dom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, renderHook, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

// On native the server address is resolved after launch. A check that ran
// before then used to cache "no setup needed" for good, so a new server was
// never offered its claim screens.

const h = vi.hoisted(() => ({
    address: null as string | null,
    listeners: new Set<() => void>(),
}))

vi.mock('@tinycld/core/lib/server-address', () => ({
    getResolvedAddress: () => h.address,
    subscribeResolvedAddress: (listener: () => void) => {
        h.listeners.add(listener)
        return () => h.listeners.delete(listener)
    },
}))

import { useNeedsSetup } from '../use-needs-setup'

function wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={new QueryClient()}>{children}</QueryClientProvider>
}

afterEach(() => {
    cleanup()
    vi.unstubAllGlobals()
    h.address = null
    h.listeners.clear()
})

describe('useNeedsSetup', () => {
    it('waits for a server address, then asks that server', async () => {
        const fetchMock = vi
            .fn()
            .mockResolvedValue({ ok: true, json: async () => ({ needsSetup: true }) })
        vi.stubGlobal('fetch', fetchMock)
        const { result } = renderHook(() => useNeedsSetup(), { wrapper })

        expect(result.current).toBeUndefined()
        expect(fetchMock).not.toHaveBeenCalled()

        act(() => {
            h.address = 'http://server'
            for (const listener of h.listeners) listener()
        })

        await waitFor(() => expect(result.current).toBe(true))
        expect(fetchMock).toHaveBeenCalledWith('http://server/api/setup/check', {
            cache: 'no-store',
        })
    })
})
