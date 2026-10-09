// @vitest-environment happy-dom
import { act, renderHook } from '@testing-library/react'
import {
    CONNECTION_ESCALATE_AFTER_MS,
    CONNECTION_INDICATOR_DELAY_MS,
    type ConnectionSignals,
    connectionNotice,
    noticeLabel,
    serverHostLabel,
} from '@tinycld/core/lib/connection-indicator'
import { useConnectivityStore } from '@tinycld/core/lib/stores/connectivity-store'
import type { SyncStatus } from 'pbtsdb'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const T0 = 1_000_000

const connected: SyncStatus = {
    realtime: { state: 'connected' },
    loads: { retrying: 0, failed: 0 },
}
function reconnectingSince(since: number): SyncStatus {
    return {
        realtime: { state: 'reconnecting', attempt: 1, nextRetryAt: since + 1000, since },
        loads: { retrying: 0, failed: 0 },
    }
}
const reconnecting = reconnectingSince(T0)

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
    isServerReachable: true,
    isSignedIn: true,
    isRealtimeEnabled: true,
    sync: connected,
}

function stateAt(signals: ConnectionSignals, now = T0) {
    return connectionNotice(signals, now).state
}

describe('connectionNotice', () => {
    it('is hidden when all is well', () => {
        expect(connectionNotice(healthy, T0)).toEqual({ state: 'hidden', escalatesAt: null })
    })

    it('says offline without a network, and never escalates it', () => {
        const signals = {
            ...healthy,
            isOnline: false,
            isServerReachable: false,
            sync: reconnecting,
        }
        expect(connectionNotice(signals, T0 + CONNECTION_ESCALATE_AFTER_MS * 10)).toEqual({
            state: 'offline',
            escalatesAt: null,
        })
    })

    it('says reconnecting for a short outage and names when it escalates', () => {
        expect(connectionNotice({ ...healthy, sync: reconnecting }, T0 + 1000)).toEqual({
            state: 'reconnecting',
            escalatesAt: T0 + CONNECTION_ESCALATE_AFTER_MS,
        })
        expect(
            stateAt({
                ...healthy,
                sync: { ...connected, loads: { retrying: 2, failingSince: T0, failed: 0 } },
            })
        ).toBe('reconnecting')
    })

    it('escalates once the outage passes the threshold', () => {
        const signals = { ...healthy, sync: reconnecting }
        expect(stateAt(signals, T0 + CONNECTION_ESCALATE_AFTER_MS - 1)).toBe('reconnecting')
        expect(stateAt(signals, T0 + CONNECTION_ESCALATE_AFTER_MS)).toBe('unreachable')
    })

    it('measures the outage from its oldest signal', () => {
        const sync: SyncStatus = {
            realtime: { state: 'reconnecting', attempt: 3, nextRetryAt: T0, since: T0 },
            loads: { retrying: 1, failingSince: T0 - 5000, failed: 0 },
        }
        expect(connectionNotice({ ...healthy, sync }, T0).escalatesAt).toBe(
            T0 - 5000 + CONNECTION_ESCALATE_AFTER_MS
        )
    })

    it('escalates at once when the health check says the server is down', () => {
        expect(connectionNotice({ ...healthy, isServerReachable: false }, T0)).toEqual({
            state: 'unreachable',
            escalatesAt: null,
        })
    })

    it('does not treat a refused load or a disabled stream as an outage', () => {
        expect(
            stateAt({ ...healthy, sync: { ...connected, loads: { retrying: 0, failed: 3 } } })
        ).toBe('hidden')
        expect(
            stateAt({ ...healthy, sync: { ...connected, realtime: { state: 'disabled' } } })
        ).toBe('hidden')
    })

    it('stays hidden when signed out or on a page with realtime turned off', () => {
        const down = { ...healthy, isOnline: false, isServerReachable: false }
        expect(stateAt({ ...down, isSignedIn: false })).toBe('hidden')
        expect(stateAt({ ...down, isRealtimeEnabled: false })).toBe('hidden')
    })
})

describe('noticeLabel', () => {
    it('names the server host when there is one', () => {
        expect(noticeLabel('unreachable', 'cloud.example.org')).toBe(
            "Can't reach cloud.example.org. Tap for options"
        )
        expect(noticeLabel('unreachable', null)).toBe("Can't reach the server. Tap for options")
        expect(noticeLabel('reconnecting', 'cloud.example.org')).toBe('Reconnecting…')
    })

    it('reads the host from a server address', () => {
        expect(serverHostLabel('https://cloud.example.org:8443/')).toBe('cloud.example.org:8443')
        expect(serverHostLabel('not a url')).toBeNull()
        expect(serverHostLabel(null)).toBeNull()
    })
})

describe('useConnectionIndicator', () => {
    beforeEach(() => {
        vi.useFakeTimers()
        vi.setSystemTime(T0)
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

    it('escalates a reconnect that outlasts the threshold, without a render to prompt it', async () => {
        const { result } = await mount()
        act(() => setStatus(reconnecting))
        act(() => vi.advanceTimersByTime(CONNECTION_INDICATOR_DELAY_MS))
        expect(result.current).toBe('reconnecting')

        act(() =>
            vi.advanceTimersByTime(CONNECTION_ESCALATE_AFTER_MS - CONNECTION_INDICATOR_DELAY_MS)
        )
        expect(result.current).toBe('unreachable')
    })

    it('escalates when the health check fails while the network is up', async () => {
        const { result } = await mount()
        act(() => useConnectivityStore.setState({ isServerReachable: false }))
        act(() => vi.advanceTimersByTime(CONNECTION_INDICATOR_DELAY_MS))
        expect(result.current).toBe('unreachable')

        act(() => useConnectivityStore.setState({ isServerReachable: true }))
        expect(result.current).toBe('hidden')
    })
})
