import { describe, expect, it, vi } from 'vitest'
import { READ_ONLY_MAX_RETRIES, retryAfterMs, withReadOnlyRetry } from './read-only-retry'

function readOnly(retryAfter = '2') {
    return new Response(JSON.stringify({ code: 'read_only', message: 'updating' }), {
        status: 503,
        headers: { 'Retry-After': retryAfter, 'Content-Type': 'application/json' },
    })
}

function ok() {
    return new Response('{}', { status: 200 })
}

describe('retryAfterMs', () => {
    it('reads seconds', () => expect(retryAfterMs('3')).toBe(3000))
    it('defaults a missing or bad header to 2 s', () => {
        expect(retryAfterMs(null)).toBe(2000)
        expect(retryAfterMs('soon')).toBe(2000)
        expect(retryAfterMs('0')).toBe(2000)
    })
    it('caps a long wait at 10 s', () => expect(retryAfterMs('600')).toBe(10000))
})

describe('withReadOnlyRetry', () => {
    it('retries a read_only 503 after Retry-After and returns the success', async () => {
        const fetchImpl = vi.fn().mockResolvedValueOnce(readOnly('1')).mockResolvedValueOnce(ok())
        const sleep = vi.fn().mockResolvedValue(undefined)
        const res = await withReadOnlyRetry(fetchImpl, sleep)('/api/x', {
            method: 'POST',
            body: '{}',
        })
        expect(res.status).toBe(200)
        expect(fetchImpl).toHaveBeenCalledTimes(2)
        expect(fetchImpl.mock.calls[1]).toEqual(['/api/x', { method: 'POST', body: '{}' }])
        expect(sleep).toHaveBeenCalledWith(1000)
    })

    it('gives up after 3 retries and returns the last 503', async () => {
        const fetchImpl = vi.fn().mockImplementation(async () => readOnly())
        const sleep = vi.fn().mockResolvedValue(undefined)
        const res = await withReadOnlyRetry(fetchImpl, sleep)('/api/x', { method: 'POST' })
        expect(res.status).toBe(503)
        expect(fetchImpl).toHaveBeenCalledTimes(1 + READ_ONLY_MAX_RETRIES)
    })

    it('does not retry another 503', async () => {
        const other = new Response(JSON.stringify({ code: 'other' }), { status: 503 })
        const fetchImpl = vi.fn().mockResolvedValue(other)
        const sleep = vi.fn()
        const res = await withReadOnlyRetry(fetchImpl, sleep)('/api/x', { method: 'POST' })
        expect(res.status).toBe(503)
        expect(fetchImpl).toHaveBeenCalledTimes(1)
        expect(sleep).not.toHaveBeenCalled()
    })

    it('does not retry a 503 whose body is not JSON', async () => {
        const fetchImpl = vi.fn().mockResolvedValue(new Response('down', { status: 503 }))
        const res = await withReadOnlyRetry(fetchImpl, vi.fn())('/api/x', { method: 'POST' })
        expect(res.status).toBe(503)
        expect(fetchImpl).toHaveBeenCalledTimes(1)
    })

    it('stops when the request was aborted', async () => {
        const controller = new AbortController()
        const fetchImpl = vi.fn().mockImplementation(async () => {
            controller.abort()
            return readOnly()
        })
        const res = await withReadOnlyRetry(fetchImpl, vi.fn().mockResolvedValue(undefined))(
            '/api/x',
            {
                method: 'POST',
                signal: controller.signal,
            }
        )
        expect(res.status).toBe(503)
        expect(fetchImpl).toHaveBeenCalledTimes(1)
    })

    it('leaves the caller a readable body', async () => {
        const fetchImpl = vi.fn().mockImplementation(async () => readOnly())
        const res = await withReadOnlyRetry(fetchImpl, vi.fn().mockResolvedValue(undefined))(
            '/api/x',
            {
                method: 'POST',
            }
        )
        await expect(res.json()).resolves.toMatchObject({ code: 'read_only' })
    })
})
