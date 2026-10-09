import { createDebouncedTrigger, isReconnect, isResume } from '@tinycld/core/lib/reload-on-resume'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

describe('isResume', () => {
    it('is a move into the foreground', () => {
        expect(isResume('background', 'active')).toBe(true)
        expect(isResume('inactive', 'active')).toBe(true)
        expect(isResume('active', 'active')).toBe(false)
        expect(isResume('active', 'background')).toBe(false)
    })
})

describe('isReconnect', () => {
    const up = { isOnline: true, isServerReachable: true }
    it('is the network or the server coming back', () => {
        expect(isReconnect({ isOnline: false, isServerReachable: true }, up)).toBe(true)
        expect(isReconnect({ isOnline: true, isServerReachable: false }, up)).toBe(true)
        expect(isReconnect(up, up)).toBe(false)
        expect(isReconnect(up, { isOnline: false, isServerReachable: true })).toBe(false)
    })
})

describe('createDebouncedTrigger', () => {
    beforeEach(() => {
        vi.useFakeTimers()
    })
    afterEach(() => {
        vi.useRealTimers()
    })

    it('runs once, after the last of a burst of triggers', async () => {
        const run = vi.fn(async () => {})
        const reload = createDebouncedTrigger(run, 1000, vi.fn())

        reload.trigger()
        await vi.advanceTimersByTimeAsync(600)
        reload.trigger()
        await vi.advanceTimersByTimeAsync(600)
        expect(run).not.toHaveBeenCalled()

        await vi.advanceTimersByTimeAsync(400)
        expect(run).toHaveBeenCalledOnce()
    })

    it('does not run once cancelled', async () => {
        const run = vi.fn(async () => {})
        const reload = createDebouncedTrigger(run, 1000, vi.fn())
        reload.trigger()
        reload.cancel()
        await vi.advanceTimersByTimeAsync(2000)
        expect(run).not.toHaveBeenCalled()
    })

    it('reports a failed run instead of rejecting unhandled', async () => {
        const error = new Error('offline')
        const onError = vi.fn()
        const reload = createDebouncedTrigger(() => Promise.reject(error), 10, onError)
        reload.trigger()
        await vi.advanceTimersByTimeAsync(10)
        expect(onError).toHaveBeenCalledWith(error)
    })
})
