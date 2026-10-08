// @vitest-environment happy-dom
import { act, renderHook } from '@testing-library/react'
import {
    CONNECTION_INDICATOR_DELAY_MS,
    type ConnectionSignals,
    connectionIndicatorState,
} from '@tinycld/core/lib/connection-indicator'
import { useConnectivityStore } from '@tinycld/core/lib/stores/connectivity-store'
import type { SyncStatus } from 'pbtsdb'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const connected: SyncStatus = {
    realtime: { state: 'connected' },
    loads: { retrying: 0, failed: 0 },
}
const reconnecting: SyncStatus = {
    realtime: { state: 'reconnecting', attempt: 1, nextRetryAt: 2000, since: 1000 },
    loads: { retrying: 0, failed: 0 },
}

const h = vi.hoisted(() => ({
    user: { id: 'u1' } as { id: string } | null,
    status: null as SyncStatus | null,
    listeners: new Set<() => void>(),
}))

vi.mock('@tinycld/core/lib/auth', () => ({
    useAuth: () => ({ user: h.user }),
}))

vi.mock('@tinycld/core/lib/pocketbase', () => ({ pb: {} }))

// pbtsdb's status for the hook: a store the test sets, read through
// useSyncExternalStore the way pbtsdb's own useSyncStatus reads its client.
vi.mock('pbtsdb', async () => {
    const { useSyncExternalStore } = await import('react')
    return {
        disconnectRealtime: vi.fn(),
        resetRealtime: vi.fn(),
        useSyncStatus: () =>
            useSyncExternalStore(
                listener => {
                    h.listeners.add(listener)
                    return () => h.listeners.delete(listener)
                },
                () => h.status
            ),
    }
})

function setStatus(status: SyncStatus) {
    h.status = status
    for (const listener of h.listeners) listener()
}

const healthy: ConnectionSignals = {
    isOnline: true,
    isSignedIn: true,
    isRealtimeEnabled: true,
    sync: connected,
}

describe('connectionIndicatorState', () => {
    it('is hidden when all is well', () => {
        expect(connectionIndicatorState(healthy)).toBe('hidden')
    })

    it('says offline without a network, before anything else', () => {
        expect(connectionIndicatorState({ ...healthy, isOnline: false, sync: reconnecting })).toBe(
            'offline'
        )
    })

    it('says reconnecting while live updates reconnect or loads retry', () => {
        expect(connectionIndicatorState({ ...healthy, sync: reconnecting })).toBe('reconnecting')
        expect(
            connectionIndicatorState({
                ...healthy,
                sync: { ...connected, loads: { retrying: 2, failingSince: 1, failed: 0 } },
            })
        ).toBe('reconnecting')
    })

    it('does not treat a refused load or a disabled stream as an outage', () => {
        expect(
            connectionIndicatorState({
                ...healthy,
                sync: { ...connected, loads: { retrying: 0, failed: 3 } },
            })
        ).toBe('hidden')
        expect(
            connectionIndicatorState({
                ...healthy,
                sync: { ...connected, realtime: { state: 'disabled' } },
            })
        ).toBe('hidden')
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
        h.status = connected
        useConnectivityStore.setState({ isOnline: true, isServerReachable: true })
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

    it('shows reconnecting only once it has lasted the delay', async () => {
        const { result } = await mount()
        act(() => setStatus(reconnecting))
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
        act(() => setStatus(reconnecting))
        act(() => vi.advanceTimersByTime(CONNECTION_INDICATOR_DELAY_MS))
        expect(result.current).toBe('reconnecting')

        act(() => setStatus(connected))
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
