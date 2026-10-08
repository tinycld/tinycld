// @vitest-environment happy-dom
import { act, renderHook } from '@testing-library/react'
import {
    CONNECTION_INDICATOR_DELAY_MS,
    type ConnectionSignals,
    connectionIndicatorState,
} from '@tinycld/core/lib/connection-indicator'
import { useConnectivityStore } from '@tinycld/core/lib/stores/connectivity-store'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const h = vi.hoisted(() => ({ user: { id: 'u1' } as { id: string } | null }))

vi.mock('@tinycld/core/lib/auth', () => ({
    useAuth: () => ({ user: h.user }),
}))

// realtime-enabled.ts reaches pbtsdb's realtime client through `pb`; the hook
// only reads the page's switch, which a plain module stands in for here.
vi.mock('@tinycld/core/lib/pocketbase', () => ({ pb: {} }))
vi.mock('pbtsdb', () => ({ disconnectRealtime: vi.fn(), resetRealtime: vi.fn() }))

const healthy: ConnectionSignals = {
    isOnline: true,
    isServerReachable: true,
    isRequestFailing: false,
    isSignedIn: true,
    isRealtimeEnabled: true,
}

describe('connectionIndicatorState', () => {
    it('is hidden when all is well', () => {
        expect(connectionIndicatorState(healthy)).toBe('hidden')
    })

    it('says offline without a network, before anything else', () => {
        expect(
            connectionIndicatorState({ ...healthy, isOnline: false, isRequestFailing: true })
        ).toBe('offline')
    })

    it('says reconnecting while requests fail or the server is unreachable', () => {
        expect(connectionIndicatorState({ ...healthy, isRequestFailing: true })).toBe(
            'reconnecting'
        )
        expect(connectionIndicatorState({ ...healthy, isServerReachable: false })).toBe(
            'reconnecting'
        )
    })

    it('stays hidden when signed out or on a page with realtime turned off', () => {
        expect(connectionIndicatorState({ ...healthy, isOnline: false, isSignedIn: false })).toBe(
            'hidden'
        )
        expect(
            connectionIndicatorState({ ...healthy, isOnline: false, isRealtimeEnabled: false })
        ).toBe('hidden')
    })
})

describe('useConnectionIndicator', () => {
    beforeEach(() => {
        vi.useFakeTimers()
        h.user = { id: 'u1' }
        useConnectivityStore.setState({
            isOnline: true,
            isServerReachable: true,
            isRequestFailing: false,
        })
    })
    afterEach(() => {
        vi.useRealTimers()
    })

    async function mount() {
        const { useConnectionIndicator } = await import(
            '@tinycld/core/lib/use-connection-indicator'
        )
        return renderHook(() => useConnectionIndicator())
    }

    it('shows a problem only once it has lasted the delay', async () => {
        const { result } = await mount()
        act(() => useConnectivityStore.setState({ isRequestFailing: true }))
        expect(result.current).toBe('hidden')

        act(() => vi.advanceTimersByTime(CONNECTION_INDICATOR_DELAY_MS - 1))
        expect(result.current).toBe('hidden')
        act(() => vi.advanceTimersByTime(1))
        expect(result.current).toBe('reconnecting')
    })

    it('never shows a blip shorter than the delay', async () => {
        const { result } = await mount()
        act(() => useConnectivityStore.setState({ isOnline: false }))
        act(() => vi.advanceTimersByTime(CONNECTION_INDICATOR_DELAY_MS / 2))
        act(() => useConnectivityStore.setState({ isOnline: true }))
        act(() => vi.advanceTimersByTime(CONNECTION_INDICATOR_DELAY_MS * 2))
        expect(result.current).toBe('hidden')
    })

    it('hides at once on recovery', async () => {
        const { result } = await mount()
        act(() => useConnectivityStore.setState({ isOnline: false }))
        act(() => vi.advanceTimersByTime(CONNECTION_INDICATOR_DELAY_MS))
        expect(result.current).toBe('offline')

        act(() => useConnectivityStore.setState({ isOnline: true }))
        expect(result.current).toBe('hidden')
    })

    it('stays hidden for a signed-out user', async () => {
        h.user = null
        const { result } = await mount()
        act(() => useConnectivityStore.setState({ isOnline: false }))
        act(() => vi.advanceTimersByTime(CONNECTION_INDICATOR_DELAY_MS * 2))
        expect(result.current).toBe('hidden')
    })
})
