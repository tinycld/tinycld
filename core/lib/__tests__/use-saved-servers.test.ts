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

// The react-native stub reports Platform.OS === 'web', so this environment IS the
// web case — which makes web the easy platform to pin here and native the one that
// needs a mock.
describe('useSavedServers on web', () => {
    // Web never calls setActiveServer (its address is window.location.origin), so
    // the stored list is empty. Without withActive the switcher would blank out on
    // the one origin it can always describe.
    it('always lists the current origin, even with nothing stored', async () => {
        const { setResolvedAddress } = await import('../server-address')
        setResolvedAddress(SERVER_A)

        const { useSavedServers } = await import('../use-saved-servers')
        const { result } = renderHook(() => useSavedServers())

        await waitFor(() => expect(result.current.servers).toHaveLength(1))
        expect(result.current.servers[0].origin).toBe(SERVER_A)
    })

    it('does not duplicate the current origin when it is already stored', async () => {
        const { setActiveServer } = await import('../servers')
        await setActiveServer(SERVER_A)

        const { setResolvedAddress } = await import('../server-address')
        setResolvedAddress(SERVER_A)

        const { useSavedServers } = await import('../use-saved-servers')
        const { result } = renderHook(() => useSavedServers())

        await waitFor(() => expect(result.current.servers).toHaveLength(1))
    })

    // A browser cannot hold a session on another origin — localStorage is
    // origin-partitioned — so the only real move is to navigate there. Going
    // through switchToServer instead would set the active pointer and then throw
    // ReloadUnavailableError, i.e. a switch that never completes.
    it('switching navigates to the target origin rather than switching in place', async () => {
        const assign = vi.fn()
        vi.stubGlobal('location', { ...window.location, assign })
        // navigateToOrgUrl traces through debug-trace, which reads the RN global
        // __DEV__. Nothing defines it under vitest, and this is the first unit
        // test to reach that path.
        vi.stubGlobal('__DEV__', false)

        const { setResolvedAddress } = await import('../server-address')
        setResolvedAddress(SERVER_A)

        const { useSavedServers } = await import('../use-saved-servers')
        const { result } = renderHook(() => useSavedServers())

        result.current.switchTo(SERVER_B)
        await waitFor(() => expect(assign).toHaveBeenCalledWith(SERVER_B))

        vi.unstubAllGlobals()
    })
})

// canSwitchInPlace is what actually forks the behaviour now that the list itself
// renders everywhere, so the native cases mock THAT rather than the support gate.
describe('useSavedServers on native', () => {
    beforeEach(() => {
        vi.doMock('../servers', async () => {
            const actual = await vi.importActual<typeof import('../servers')>('../servers')
            return { ...actual, canSwitchInPlace: () => true }
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
