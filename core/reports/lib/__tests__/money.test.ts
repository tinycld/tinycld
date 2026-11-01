import { describe, expect, it } from 'vitest'
import { formatMoney, MAX_STORED_AMOUNT, roundHalfAwayFromZero } from '../money'

describe('formatMoney', () => {
    it('formats whole cents with the currency layout', () => {
        expect(formatMoney(123456, 'USD')).toBe('$1,234.56')
        expect(formatMoney(-5, 'USD')).toBe('-$0.05')
        expect(formatMoney(123456, 'EUR')).toBe('€1,234.56')
    })

    it('is exact at 2^53-1', () => {
        expect(MAX_STORED_AMOUNT).toBe(9007199254740991)
        expect(formatMoney(9007199254740991, 'USD')).toBe('$90,071,992,547,409.91')
    })

    it('never shows a negative zero', () => {
        expect(formatMoney(-0.4, 'USD')).toBe('$0.00')
    })

    it('falls back to plain digits for no or an unknown currency', () => {
        expect(formatMoney(123456, '')).toBe('1,234.56')
        expect(formatMoney(123456, 'NOT A CODE')).toBe('1,234.56 NOT A CODE')
    })

    it('shows a dash for a value that is not a number', () => {
        expect(formatMoney(Number.NaN, 'USD')).toBe('—')
    })
})

describe('roundHalfAwayFromZero', () => {
    it('rounds halves away from zero', () => {
        expect(roundHalfAwayFromZero(2.5)).toBe(3)
        expect(roundHalfAwayFromZero(-2.5)).toBe(-3)
        // 10.005 * 100 is 1000.4999999999999 in float64; a person reading 10.005 expects 1001.
        expect(roundHalfAwayFromZero(10.005 * 100)).toBe(1001)
    })
})
