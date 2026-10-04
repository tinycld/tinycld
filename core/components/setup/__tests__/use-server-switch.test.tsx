// @vitest-environment happy-dom

// A finished job does not mean the new server answers yet: the old server
// keeps serving until the new one is ready. The hook must hold the reload
// offer until /api/version reports a release other than the one served when
// the job started, and give up with an "unconfirmed" verdict after its limit.
// fetch is the network boundary here (same posture as
// use-install-progress.test.ts); serverFetch and react-query are the real ones.
// Timers are fake: the poll interval and the limit are moved by hand, so no
// assertion depends on how fast the machine runs.

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, renderHook } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { setResolvedAddress } from '../../../lib/server-address'
import type { OperationStatus } from '../use-install-progress'
import { useServerSwitch } from '../use-server-switch'

const TIMING = { intervalMs: 1_000, timeoutMs: 5_000 }

function wrapper() {
    const client = new QueryClient()
    return ({ children }: { children: ReactNode }) => (
        <QueryClientProvider client={client}>{children}</QueryClientProvider>
    )
}

// Serves `served.id` on every request; a test changes it to model the switch.
function servedRelease(id: string) {
    const served = { id }
    const fetchFn = vi.fn(async () => ({
        ok: true,
        status: 200,
        json: async () => ({ releaseId: served.id }),
    }))
    vi.stubGlobal('fetch', fetchFn)
    return { served, fetchFn }
}

function renderSwitch(initialStatus: OperationStatus) {
    return renderHook(
        ({ status }: { status: OperationStatus }) => useServerSwitch(true, 'job_1', status, TIMING),
        { wrapper: wrapper(), initialProps: { status: initialStatus } }
    )
}

// Runs every timer due within ms, inside act so the hook's state updates are
// applied before the next assertion. A 0 ms timer set while the fake clock
// ticks is due 1 ms later (fake-timers does this so a timer that sets itself
// again cannot loop forever), and react-query tells its observers on such a
// timer, so the last 1 ms step delivers what the timers in ms produced.
async function advance(ms: number) {
    await act(async () => {
        await vi.advanceTimersByTimeAsync(ms)
        await vi.advanceTimersByTimeAsync(1)
    })
}

describe('useServerSwitch', () => {
    beforeEach(() => {
        vi.useFakeTimers()
        setResolvedAddress('http://localhost:8090')
    })
    afterEach(() => {
        setResolvedAddress(null)
        vi.unstubAllGlobals()
        vi.useRealTimers()
    })

    it('waits while the job runs', async () => {
        const { fetchFn } = servedRelease('A')
        const { result } = renderSwitch('running')

        await advance(0)
        expect(fetchFn).toHaveBeenCalledTimes(1)
        await advance(TIMING.timeoutMs * 2)
        expect(fetchFn).toHaveBeenCalledTimes(1)
        expect(result.current).toBe('waiting')
    })

    it('waits while the server still reports the release it served at the start', async () => {
        const { served, fetchFn } = servedRelease('A')
        const { result, rerender } = renderSwitch('running')
        await advance(0)
        expect(fetchFn).toHaveBeenCalledTimes(1)

        rerender({ status: 'success' })
        await advance(0)
        await advance(TIMING.intervalMs)

        expect(fetchFn).toHaveBeenCalledTimes(3)
        expect(result.current).toBe('waiting')
        served.id = 'B'
        await advance(TIMING.intervalMs)
        expect(result.current).toBe('ready')
    })

    it('is ready once the server reports a different release', async () => {
        const { served, fetchFn } = servedRelease('A')
        const { result, rerender } = renderSwitch('running')
        await advance(0)
        expect(fetchFn).toHaveBeenCalledTimes(1)
        served.id = 'B'
        rerender({ status: 'success' })

        await advance(0)
        expect(result.current).toBe('ready')
    })

    it('is unconfirmed when the release does not change within the limit', async () => {
        servedRelease('A')
        const { result, rerender } = renderSwitch('running')
        await advance(0)
        rerender({ status: 'success' })

        await advance(TIMING.timeoutMs - TIMING.intervalMs)
        expect(result.current).toBe('waiting')
        await advance(TIMING.intervalMs)
        expect(result.current).toBe('unconfirmed')
    })

    it('is unconfirmed at once when the starting release is unknown', async () => {
        vi.stubGlobal(
            'fetch',
            vi.fn(async () => ({ ok: false, status: 500, json: async () => ({}) }))
        )
        const { result, rerender } = renderSwitch('running')
        await advance(0)
        rerender({ status: 'success' })

        await advance(0)
        expect(result.current).toBe('unconfirmed')
    })

    it('does not poll after a failed job', async () => {
        const { served, fetchFn } = servedRelease('A')
        const { result, rerender } = renderSwitch('running')
        await advance(0)
        expect(fetchFn).toHaveBeenCalledTimes(1)
        served.id = 'B'

        rerender({ status: 'failed' })

        await advance(TIMING.timeoutMs * 2)
        expect(fetchFn).toHaveBeenCalledTimes(1)
        expect(result.current).toBe('waiting')
    })
})
