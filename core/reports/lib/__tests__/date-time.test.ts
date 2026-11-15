import { describe, expect, it } from 'vitest'
import {
    dateTimeText,
    formatDateTime,
    isTwelveHour,
    readerTimeZone,
    timeZoneWords,
} from '../date-time'

const AT = '2026-11-14 22:07:00.000Z'
const CHICAGO = 'America/Chicago'

describe('formatDateTime', () => {
    it("writes an instant as the day and time on the zone's wall clock", () => {
        expect(formatDateTime(AT, CHICAGO, true)).toBe('Nov 14, 2026, 4:07 PM')
        expect(formatDateTime(AT, CHICAGO, false)).toBe('Nov 14, 2026, 16:07')
    })

    it('writes local midnight as 12:00 AM, or 00:00', () => {
        expect(formatDateTime('2026-11-15 06:00:00.000Z', CHICAGO, true)).toBe(
            'Nov 15, 2026, 12:00 AM'
        )
        expect(formatDateTime('2026-11-15 06:00:00.000Z', CHICAGO, false)).toBe(
            'Nov 15, 2026, 00:00'
        )
    })

    it('follows the clock change at the end of daylight time', () => {
        // Nov 1 2026: 06:30Z is 1:30 AM CDT, and 07:30Z is 1:30 AM CST an hour later.
        expect(formatDateTime('2026-11-01 06:30:00.000Z', CHICAGO, true)).toBe(
            'Nov 1, 2026, 1:30 AM'
        )
        expect(formatDateTime('2026-11-01 07:30:00.000Z', CHICAGO, true)).toBe(
            'Nov 1, 2026, 1:30 AM'
        )
        expect(formatDateTime('2026-11-01 04:59:59.000Z', CHICAGO, true)).toBe(
            'Oct 31, 2026, 11:59 PM'
        )
    })

    it('writes an unknown zone as UTC, as the server reads it', () => {
        expect(formatDateTime(AT, 'Not/AZone', true)).toBe('Nov 14, 2026, 10:07 PM')
    })

    it('reads an instant written with a T', () => {
        expect(formatDateTime('2026-11-14T22:07:00Z', CHICAGO, true)).toBe('Nov 14, 2026, 4:07 PM')
    })

    it('shows a bare day as the short day and any other text as it is', () => {
        expect(formatDateTime('2026-11-14', CHICAGO, true)).toBe('Nov 14, 2026')
        expect(formatDateTime('soon', CHICAGO, true)).toBe('soon')
        expect(formatDateTime('2026-13-40 22:07:00.000Z', CHICAGO, true)).toBe(
            '2026-13-40 22:07:00.000Z'
        )
    })
})

describe('dateTimeText', () => {
    it("writes the instant in the zone given, in the device's hour cycle", () => {
        // The test runtime's locale is en-US, a 12-hour locale (R17).
        expect(dateTimeText(AT, CHICAGO)).toBe('Nov 14, 2026, 4:07 PM')
    })

    it("defaults to the reader's zone", () => {
        expect(dateTimeText(AT)).toBe(dateTimeText(AT, readerTimeZone()))
    })
})

describe('readerTimeZone', () => {
    it("is the device's IANA zone, UTC when it has none", () => {
        expect(readerTimeZone()).toBe(new Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC')
    })
})

describe('isTwelveHour', () => {
    it("reads the locale's hour cycle", () => {
        expect(isTwelveHour({ hourCycle: 'h12' })).toBe(true)
        expect(isTwelveHour({ hourCycle: 'h11' })).toBe(true)
        expect(isTwelveHour({ hourCycle: 'h23' })).toBe(false)
        expect(isTwelveHour({ hourCycle: 'h24' })).toBe(false)
    })

    // A Hermes build may leave out hourCycle and still give hour12 (R4).
    it('falls back to hour12, then to the 12-hour clock', () => {
        expect(isTwelveHour({ hour12: false })).toBe(false)
        expect(isTwelveHour({ hour12: true })).toBe(true)
        expect(isTwelveHour({})).toBe(true)
    })
})

describe('timeZoneWords', () => {
    it('names a zone by its generic name', () => {
        expect(timeZoneWords(CHICAGO)).toBe('Central Time')
    })

    it('names UTC for UTC and for a zone it does not know', () => {
        expect(timeZoneWords('UTC')).toBe('UTC')
        expect(timeZoneWords('Etc/UTC')).toBe('UTC')
        expect(timeZoneWords('Not/AZone')).toBe('UTC')
    })

    it("defaults to the reader's zone", () => {
        expect(timeZoneWords()).toBe(timeZoneWords(readerTimeZone()))
    })
})
