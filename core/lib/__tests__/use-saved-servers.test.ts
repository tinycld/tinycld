// @vitest-environment happy-dom

import AsyncStorage from '@react-native-async-storage/async-storage'
import { renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('../pocketbase', () => ({
    pb: { realtime: { unsubscribe: vi.fn() }, cancelAllRequests: vi.fn() },
    disconnectServer: vi.fn(),
    resetSessionState: vi.fn(),
}))

const SERVER_A = 'https://a.example.com'
const SERVER_B = 'https://b.example.com'

beforeEach(async () => {
    await AsyncStorage.clear()
    vi.resetModules()
})

// The react-native stub reports Platform.OS === 'web', so the DEFAULT state in
// this environment is the unsupported one — which makes it the easy case to pin.
describe('useSavedServers on an unsupported platform', () => {
    it('returns an inert state rather than making callers remember the gate', async () => {
        const { useSavedServers } = await import('../use-saved-servers')
        const { result } = renderHook(() => useSavedServers())

        expect(result.current.servers).toEqual([])
        expect(result.current.activeOrigin).toBeNull()
        expect(result.current.canReload).toBe(false)
    })

    // Forgetting the gate on web would push /connect?mode=add to connect.web.tsx,
    // which has no add mode — so the inert callbacks must be genuinely inert.
    it('hands back no-op callbacks', async () => {
        const { useSavedServers } = await import('../use-saved-servers')
        const { result } = renderHook(() => useSavedServers())

        expect(() => {
            result.current.add()
            result.current.switchTo(SERVER_A)
            result.current.remove(SERVER_A)
        }).not.toThrow()
    })

    it('does not read storage', async () => {
        const spy = vi.spyOn(AsyncStorage, 'getItem')
        const { useSavedServers } = await import('../use-saved-servers')
        renderHook(() => useSavedServers())

        await waitFor(() => expect(spy).not.toHaveBeenCalled())
        spy.mockRestore()
    })
})

describe('useSavedServers when supported', () => {
    beforeEach(() => {
        vi.doMock('../servers', async () => {
            const actual = await vi.importActual<typeof import('../servers')>('../servers')
            return { ...actual, isSavedServersSupported: () => true }
        })
    })

    // Without the seed, a length-gated surface pops in on the second frame every
    // time it opens: activeOrigin is a sync read while the list is async.
    it('seeds the first render with the active server', async () => {
        const { setResolvedAddress } = await import('../server-address')
        setResolvedAddress(SERVER_A)

        const { useSavedServers } = await import('../use-saved-servers')
        const { result } = renderHook(() => useSavedServers())

        // Synchronously, before the AsyncStorage read resolves.
        expect(result.current.servers).toHaveLength(1)
        expect(result.current.servers[0].origin).toBe(SERVER_A)
        expect(result.current.servers[0].label).toBe('a.example.com')
    })

    it('replaces the seed with the stored list once it loads', async () => {
        const { setActiveServer } = await import('../servers')
        await setActiveServer(SERVER_A)
        await setActiveServer(SERVER_B)

        const { setResolvedAddress } = await import('../server-address')
        setResolvedAddress(SERVER_B)

        const { useSavedServers } = await import('../use-saved-servers')
        const { result } = renderHook(() => useSavedServers())

        await waitFor(() => expect(result.current.servers).toHaveLength(2))
        expect(result.current.servers.map(s => s.origin)).toEqual([SERVER_A, SERVER_B])
    })

    it('seeds empty when no address is resolved yet', async () => {
        const { setResolvedAddress } = await import('../server-address')
        setResolvedAddress(null)

        const { useSavedServers } = await import('../use-saved-servers')
        const { result } = renderHook(() => useSavedServers())

        expect(result.current.servers).toEqual([])
    })

    // In this environment there is no native reload, so canReload must be false —
    // which is what drives the "requires a restart in this build" notice.
    it('reports canReload from isReloadAvailable', async () => {
        const { setResolvedAddress } = await import('../server-address')
        setResolvedAddress(SERVER_A)

        const { useSavedServers } = await import('../use-saved-servers')
        const { result } = renderHook(() => useSavedServers())

        expect(result.current.canReload).toBe(false)
    })
})
