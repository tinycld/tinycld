import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { serverFetch } from './server-fetch'

function readOnly(retryAfter = '1') {
    return new Response(JSON.stringify({ code: 'read_only', message: 'updating' }), {
        status: 503,
        headers: { 'Retry-After': retryAfter, 'Content-Type': 'application/json' },
    })
}

function ok() {
    return new Response('{}', { status: 200 })
}

describe('serverFetch', () => {
    beforeEach(() => {
        vi.useFakeTimers()
    })

    afterEach(() => {
        vi.useRealTimers()
        vi.restoreAllMocks()
        vi.unstubAllGlobals()
    })

    it('retries a read_only 503 and returns the eventual success', async () => {
        const responses = [readOnly(), ok()]
        const fetchSpy = vi.spyOn(globalThis, 'fetch').mockImplementation(async () => {
            const next = responses.shift()
            if (!next) throw new Error('no more responses queued')
            return next
        })

        const resPromise = serverFetch('/api/x', { method: 'POST' })
        await vi.advanceTimersByTimeAsync(1000)
        const res = await resPromise

        expect(res.status).toBe(200)
        expect(fetchSpy).toHaveBeenCalledTimes(2)
    })

    it('reads globalThis.fetch per call rather than capturing it at module load', async () => {
        const first = vi.fn().mockResolvedValue(ok())
        vi.stubGlobal('fetch', first)

        await serverFetch('/api/x')

        expect(first).toHaveBeenCalledTimes(1)
    })
})
