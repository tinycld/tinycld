import { describe, expect, it } from 'vitest'
import { bucketWords, dayWords, isIsoDay, rangeChipText, rangeWords, shortDay } from '../dates'

describe('isIsoDay', () => {
    it('accepts real days from 1900', () => {
        expect(isIsoDay('2026-02-28')).toBe(true)
        expect(isIsoDay('2024-02-29')).toBe(true)
        expect(isIsoDay('1900-01-01')).toBe(true)
    })

    it('refuses what is not a real day', () => {
        for (const text of [
            '2026-02-29',
            '2026-13-01',
            '2026-00-10',
            '1899-12-31',
            '2026-1-01',
            'today',
            '',
        ])
            expect(isIsoDay(text)).toBe(false)
    })
})

describe('day and range words', () => {
    it('writes a day out', () => {
        expect(dayWords('2026-10-05')).toBe('October 5, 2026')
        expect(shortDay('2026-10-05')).toBe('Oct 5, 2026')
    })

    it('drops the first year when both ends share it', () => {
        expect(rangeWords('2026-10-01', '2026-10-25')).toBe('October 1 – October 25, 2026')
        expect(rangeWords('2025-07-01', '2026-06-30')).toBe('July 1, 2025 – June 30, 2026')
        expect(rangeChipText('2026-01-01', '2026-10-25')).toBe('Jan 1 – Oct 25, 2026')
    })

    it('writes a one-day range as one day', () => {
        expect(rangeWords('2026-10-05', '2026-10-05')).toBe('October 5, 2026')
    })
})

describe('bucketWords', () => {
    it('writes each bucket key in words', () => {
        expect(bucketWords('month', '2026-01')).toBe('Jan 2026')
        expect(bucketWords('month', '2026-12')).toBe('Dec 2026')
        expect(bucketWords('quarter', '2026-Q1')).toBe('Q1 2026')
        expect(bucketWords('year', '2026')).toBe('2026')
        // The engine keys a fiscal year by the year it ends in, as a number.
        expect(bucketWords('fiscalYear', 2026)).toBe('FY 2026')
        expect(bucketWords('day', '2026-01-05')).toBe('Jan 5, 2026')
    })

    it('keeps a key it cannot read', () => {
        expect(bucketWords('month', '2026-13')).toBe('2026-13')
        expect(bucketWords('month', '2026-00')).toBe('2026-00')
        expect(bucketWords('month', 'soon')).toBe('soon')
        expect(bucketWords('quarter', '2026-Q5')).toBe('2026-Q5')
        expect(bucketWords('fiscalYear', 'FY')).toBe('FY')
        expect(bucketWords('day', '2026-02-30')).toBe('2026-02-30')
    })
})
