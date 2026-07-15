// @vitest-environment happy-dom
import { renderHook, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

// The gate resolves the PocketBase server address before mounting the provider
// tree, and redirects address-less routes to /connect. The demo route pins the
// hosted server itself, so it must be exempt from that redirect AND get the
// address seeded synchronously — otherwise a universal-link demo open on a fresh
// install blank-screens /p/demo before it can log in. These tests guard both.

const DEMO_SERVER = 'https://tinycld.org'

// The gate guards the demo-server seed with !__DEV__ (see the hook). Pin it false
// so these tests exercise the production universal-link path.
vi.stubGlobal('__DEV__', false)

const replace = vi.fn()
vi.mock('expo-router', () => ({ router: { replace: (...a: unknown[]) => replace(...a) } }))

// A mutable in-memory server-address module: no env, no cache by default, so the
// gate lands in 'unresolved' unless a route seeds an address. vars are hoisted
// with the vi.mock factory, so the literal is inlined here rather than referenced.
const state = vi.hoisted(() => ({ resolved: null as string | null, cached: null as string | null }))
vi.mock('@tinycld/core/lib/server-address', () => ({
    DEMO_SERVER: 'https://tinycld.org',
    resolveEnvAddress: () => null,
    getResolvedAddress: () => state.resolved,
    setResolvedAddress: (a: string | null) => {
        state.resolved = a
    },
    readCached: async () => state.cached,
    subscribeResolvedAddress: () => () => {},
}))

// The gate dynamically imports Providers once an address resolves; stub it so the
// import doesn't pull the real (native-heavy) module into the test.
vi.mock('@tinycld/core/components/Providers', () => ({ Providers: () => null }))

import { useServerAddressGate } from '../../lib/use-server-address-gate'

beforeEach(() => {
    state.resolved = null
    state.cached = null
    replace.mockClear()
})

afterEach(() => {
    vi.restoreAllMocks()
})

test('/p/demo seeds the hosted demo server and does not redirect to /connect', async () => {
    renderHook(() => useServerAddressGate('/p/demo'))
    // Seeded synchronously in the state initializer.
    expect(state.resolved).toBe(DEMO_SERVER)
    // Address is now resolved, so no redirect fires.
    await waitFor(() => {})
    expect(replace).not.toHaveBeenCalled()
})

test('an address-less non-exempt route redirects to /connect with backTo', async () => {
    const { result } = renderHook(() => useServerAddressGate('/a/acme/mail'))
    await waitFor(() => expect(result.current.status).toBe('unresolved'))
    expect(replace).toHaveBeenCalledWith('/connect?backTo=%2Fa%2Facme%2Fmail')
})

test('/connect is exempt from the redirect even with no address', async () => {
    const { result } = renderHook(() => useServerAddressGate('/connect'))
    await waitFor(() => expect(result.current.status).toBe('unresolved'))
    expect(replace).not.toHaveBeenCalled()
})
