import { describe, expect, it } from 'vitest'
import { isStale, POW_MAX_AGE_MS, STAMP_MAX_AGE_MS } from '../freshness'

describe('isStale', () => {
    it('keeps a proof of work for less than its 10 minute life, with a margin', () => {
        expect(POW_MAX_AGE_MS).toBeLessThan(10 * 60 * 1000)
        expect(isStale(0, POW_MAX_AGE_MS - 1, POW_MAX_AGE_MS)).toBe(false)
        expect(isStale(0, POW_MAX_AGE_MS + 1, POW_MAX_AGE_MS)).toBe(true)
    })

    it('renews a stamp before its 2 hour life ends', () => {
        expect(STAMP_MAX_AGE_MS).toBeLessThan(2 * 60 * 60 * 1000)
        expect(isStale(1000, 1000 + STAMP_MAX_AGE_MS + 1, STAMP_MAX_AGE_MS)).toBe(true)
    })
})
