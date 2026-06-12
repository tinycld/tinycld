// @vitest-environment happy-dom

// Drives useInstallProgress through its DURABLE POLL path: a stubbed EventSource
// reports the stream gone immediately (the restart killed it), which arms the
// job-status poll; a mocked fetch then returns the terminal pkg_install_log row.
// This is the post-restart seam the poll exists to cover — including the
// 'rolled_back' outcome the server writes after a health-check rollback.

import { renderHook, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { setResolvedAddress } from '../../../lib/server-address'
import { useInstallProgress } from '../use-install-progress'

// A minimal EventSource stub that is "CLOSED before any terminal event" — i.e.
// the restart dropped it — so the hook's error handler fires onStreamGone and
// arms the poll. It never emits a 'complete' event.
class ClosedEventSource {
    static CLOSED = 2
    static CONNECTING = 0
    static OPEN = 1
    readyState = ClosedEventSource.CLOSED
    private listeners: Record<string, ((ev: MessageEvent) => void)[]> = {}
    constructor(public url: string) {
        // Fire 'error' on the next tick so the hook subscribes first.
        setTimeout(() => {
            for (const cb of this.listeners.error ?? []) cb(new MessageEvent('error'))
        }, 0)
    }
    addEventListener(type: string, cb: (ev: MessageEvent) => void) {
        if (!this.listeners[type]) this.listeners[type] = []
        this.listeners[type].push(cb)
    }
    close() {}
}

function mockJobStatus(status: string, error = '') {
    return vi.fn(async () => ({
        ok: true,
        json: async () => ({ status, error }),
    })) as unknown as typeof fetch
}

describe('useInstallProgress durable poll', () => {
    beforeEach(() => {
        vi.stubGlobal('EventSource', ClosedEventSource)
        // The hook builds its URLs from PB_SERVER_ADDR, a lazy proxy that throws
        // until the _layout.tsx gate resolves the address — satisfy that gate.
        setResolvedAddress('http://localhost:8090')
    })
    afterEach(() => {
        setResolvedAddress(null)
        vi.unstubAllGlobals()
        vi.restoreAllMocks()
    })

    it('resolves to failed when the job rolled back', async () => {
        vi.stubGlobal('fetch', mockJobStatus('rolled_back'))
        const onSuccess = vi.fn()

        const { result } = renderHook(() => useInstallProgress(true, 'job_1', 'tok', onSuccess))

        await waitFor(() => expect(result.current.status).toBe('failed'))
        expect(onSuccess).not.toHaveBeenCalled()
        expect(result.current.error).toMatch(/roll(ed)? back/i)
    })

    it('resolves to success on a success outcome', async () => {
        vi.stubGlobal('fetch', mockJobStatus('success'))
        const onSuccess = vi.fn()

        const { result } = renderHook(() => useInstallProgress(true, 'job_2', 'tok', onSuccess))

        await waitFor(() => expect(result.current.status).toBe('success'))
        expect(onSuccess).toHaveBeenCalledTimes(1)
    })
})
