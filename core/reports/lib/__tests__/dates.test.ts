import { describe, expect, it } from 'vitest'
import { dayWords, isIsoDay, rangeChipText, rangeWords, shortDay } from '../dates'

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
