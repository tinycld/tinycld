import { describe, expect, it } from 'vitest'
import { nextAttempt, shouldRetryOnTimeout } from './lazy-sidebar-retry'

describe('nextAttempt', () => {
    it('increments while under the cap', () => {
        expect(nextAttempt(0, 3)).toBe(1)
        expect(nextAttempt(1, 3)).toBe(2)
        expect(nextAttempt(2, 3)).toBe(3)
    })

    it('stops at the cap so a broken chunk does not loop forever', () => {
        expect(nextAttempt(3, 3)).toBe(3)
        expect(nextAttempt(4, 3)).toBe(4) // already past cap → unchanged, never grows
    })

    it('honors a zero-retry cap (no recovery)', () => {
        expect(nextAttempt(0, 0)).toBe(0)
    })
})

describe('shouldRetryOnTimeout', () => {
    it('retries when still in the skeleton and under the cap', () => {
        expect(shouldRetryOnTimeout(false, 0, 3)).toBe(true)
        expect(shouldRetryOnTimeout(false, 2, 3)).toBe(true)
    })

    it('never tears down a committed sidebar', () => {
        // mounted=true → the real sidebar is showing; the watchdog must stand down.
        expect(shouldRetryOnTimeout(true, 0, 3)).toBe(false)
        expect(shouldRetryOnTimeout(true, 2, 3)).toBe(false)
    })

    it('gives up once retries are exhausted', () => {
        expect(shouldRetryOnTimeout(false, 3, 3)).toBe(false)
        expect(shouldRetryOnTimeout(false, 4, 3)).toBe(false)
    })
})
