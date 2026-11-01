import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createRerunScheduler } from '../rerun-scheduler'

// A run that stays in flight until the test settles it.
function heldRuns() {
    const settles: (() => void)[] = []
    const run = vi.fn(
        () =>
            new Promise<void>(resolve => {
                settles.push(resolve)
            })
    )
    // The run's promise, its catch and its finally each take a microtask.
    const settle = async () => {
        settles.shift()?.()
        for (let i = 0; i < 5; i += 1) await Promise.resolve()
    }
    return { run, settle }
}

describe('createRerunScheduler', () => {
    beforeEach(() => vi.useFakeTimers())
    afterEach(() => vi.useRealTimers())

    it('runs once after a burst goes quiet', () => {
        const { run } = heldRuns()
        const scheduler = createRerunScheduler({ delayMs: 250, run, onError: vi.fn() })
        scheduler.signal()
        vi.advanceTimersByTime(200)
        scheduler.signal()
        vi.advanceTimersByTime(200)
        expect(run).not.toHaveBeenCalled()
        vi.advanceTimersByTime(50)
        expect(run).toHaveBeenCalledTimes(1)
    })

    it('queues exactly one follow-up for messages that arrive during a run', async () => {
        const { run, settle } = heldRuns()
        const scheduler = createRerunScheduler({ delayMs: 250, run, onError: vi.fn() })
        scheduler.signal()
        vi.advanceTimersByTime(250)
        scheduler.signal()
        scheduler.signal()
        scheduler.signal()
        vi.advanceTimersByTime(1000)
        expect(run).toHaveBeenCalledTimes(1)
        await settle()
        vi.advanceTimersByTime(250)
        expect(run).toHaveBeenCalledTimes(2)
        await settle()
        vi.advanceTimersByTime(1000)
        expect(run).toHaveBeenCalledTimes(2)
    })

    it('runs nothing after it is disposed', () => {
        const { run } = heldRuns()
        const scheduler = createRerunScheduler({ delayMs: 250, run, onError: vi.fn() })
        scheduler.signal()
        scheduler.dispose()
        scheduler.signal()
        vi.advanceTimersByTime(1000)
        expect(run).not.toHaveBeenCalled()
    })

    it('reports a failed run and keeps working', async () => {
        const onError = vi.fn()
        const run = vi.fn().mockRejectedValueOnce(new Error('boom')).mockResolvedValue(undefined)
        const scheduler = createRerunScheduler({ delayMs: 250, run, onError })
        scheduler.signal()
        await vi.advanceTimersByTimeAsync(250)
        for (let i = 0; i < 5; i += 1) await Promise.resolve()
        expect(onError).toHaveBeenCalledWith(expect.objectContaining({ message: 'boom' }))
        scheduler.signal()
        await vi.advanceTimersByTimeAsync(250)
        expect(run).toHaveBeenCalledTimes(2)
    })
})
