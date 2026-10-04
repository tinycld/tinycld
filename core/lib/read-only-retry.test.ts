import { describe, expect, it, vi } from 'vitest'
import {
    abortableWait,
    isRetryableBody,
    READ_ONLY_MAX_RETRIES,
    retryAfterMs,
    withReadOnlyRetry,
} from './read-only-retry'

function readOnly(retryAfter = '2') {
    return new Response(JSON.stringify({ code: 'read_only', message: 'updating' }), {
        status: 503,
        headers: { 'Retry-After': retryAfter, 'Content-Type': 'application/json' },
    })
}

function retryLater(retryAfter = '2') {
    return new Response(JSON.stringify({ code: 'retry_later', message: 'starting up' }), {
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

describe('abortableWait', () => {
    it('resolves without waiting out the timer when already aborted', async () => {
        const controller = new AbortController()
        controller.abort()
        const start = Date.now()
        await abortableWait(10000, controller.signal)
        expect(Date.now() - start).toBeLessThan(100)
    })

    it('resolves as soon as the signal aborts, not after the full delay', async () => {
        const controller = new AbortController()
        const promise = abortableWait(10000, controller.signal)
        setTimeout(() => controller.abort(), 5)
        const start = Date.now()
        await promise
        expect(Date.now() - start).toBeLessThan(1000)
    })

    it('resolves after the delay when never aborted', async () => {
        await expect(abortableWait(1)).resolves.toBeUndefined()
    })
})

describe('isRetryableBody', () => {
    it('matches read_only', () => {
        expect(isRetryableBody(503, { code: 'read_only' })).toBe(true)
    })
    it('matches retry_later', () => {
        expect(isRetryableBody(503, { code: 'retry_later' })).toBe(true)
    })
    it('rejects other codes', () => {
        expect(isRetryableBody(503, { code: 'other' })).toBe(false)
    })
    it('rejects a non-503 status', () => {
        expect(isRetryableBody(500, { code: 'read_only' })).toBe(false)
    })
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
        expect(sleep).toHaveBeenCalledWith(1000, undefined)
    })

    it('retries a retry_later 503 after Retry-After and returns the success', async () => {
        const fetchImpl = vi.fn().mockResolvedValueOnce(retryLater('1')).mockResolvedValueOnce(ok())
        const sleep = vi.fn().mockResolvedValue(undefined)
        const res = await withReadOnlyRetry(fetchImpl, sleep)('/api/x', {
            method: 'POST',
            body: '{}',
        })
        expect(res.status).toBe(200)
        expect(fetchImpl).toHaveBeenCalledTimes(2)
        expect(sleep).toHaveBeenCalledWith(1000, undefined)
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

    it('uses the real abortable wait by default, ending early on abort instead of refetching', async () => {
        const controller = new AbortController()
        const fetchImpl = vi.fn().mockResolvedValue(readOnly('10'))
        const promise = withReadOnlyRetry(fetchImpl)('/api/x', {
            method: 'POST',
            signal: controller.signal,
        })
        setTimeout(() => controller.abort(), 5)
        const start = Date.now()
        const res = await promise
        expect(Date.now() - start).toBeLessThan(1000)
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
