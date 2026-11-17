import { afterEach, describe, expect, it, vi } from 'vitest'
import {
    dateTimeText,
    formatDateTime,
    hour24,
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

    it('writes the names the server reads as UTC as UTC', () => {
        for (const zone of ['', 'Local', 'GMT']) {
            expect(formatDateTime(AT, zone, true)).toBe('Nov 14, 2026, 10:07 PM')
        }
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

describe('hour24', () => {
    const parts = (hour: string, dayPeriod?: string): Intl.DateTimeFormatPart[] => [
        { type: 'hour', value: hour },
        ...(dayPeriod ? [{ type: 'dayPeriod' as const, value: dayPeriod }] : []),
    ]

    it('reads a day period when the engine ignores the hour cycle', () => {
        expect(hour24(parts('12', 'AM'))).toBe(0)
        expect(hour24(parts('12', 'PM'))).toBe(12)
        expect(hour24(parts('4', 'PM'))).toBe(16)
    })

    it('reads hour 24 with no period as midnight', () => {
        expect(hour24(parts('24'))).toBe(0)
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
        expect(timeZoneWords('')).toBe('UTC')
        expect(timeZoneWords('Local')).toBe('UTC')
        expect(timeZoneWords('GMT')).toBe('UTC')
    })

    describe('fallbacks', () => {
        const RealDateTimeFormat = Intl.DateTimeFormat

        // The code under test calls the constructor with `new`, and a spy can
        // only be constructed when its implementation is a function declaration
        // or a class: an arrow function throws "is not a constructor".
        function spyOnDateTimeFormat(
            implementation: (
                locale?: Intl.LocalesArgument,
                options?: Intl.DateTimeFormatOptions
            ) => Intl.DateTimeFormat
        ) {
            return vi
                .spyOn(Intl, 'DateTimeFormat')
                .mockImplementation(implementation as unknown as typeof Intl.DateTimeFormat)
        }

        // Wraps the real constructor; `override` may replace what one style answers.
        // timeZoneWords caches per zone, so each test names a zone of its own.
        function spyOnZoneNames(
            override: (style: string) => Intl.DateTimeFormatPart[] | 'throw' | undefined
        ) {
            function ZoneNames(
                locale?: Intl.LocalesArgument,
                options?: Intl.DateTimeFormatOptions
            ) {
                const real = new RealDateTimeFormat(locale, options)
                const style = options?.timeZoneName
                if (!style) return real
                const answer = override(style)
                if (answer === 'throw') throw new RangeError('invalid timeZoneName')
                if (!answer) return real
                return { formatToParts: () => answer } as unknown as Intl.DateTimeFormat
            }
            return spyOnDateTimeFormat(ZoneNames)
        }

        afterEach(() => {
            vi.restoreAllMocks()
        })

        it('makes the long name generic when the generic style is refused', () => {
            spyOnZoneNames(style => (style === 'longGeneric' ? 'throw' : undefined))
            expect(timeZoneWords('America/Denver')).toBe('Mountain Time')
        })

        it("falls back to the zone's city when both styles give a GMT offset", () => {
            spyOnZoneNames(() => [{ type: 'timeZoneName', value: 'GMT-06:00' }])
            expect(timeZoneWords('America/Regina')).toBe('Regina time')
        })

        it('does not take a short name for words', () => {
            spyOnZoneNames(() => [{ type: 'timeZoneName', value: 'CST' }])
            expect(timeZoneWords('America/Winnipeg')).toBe('Winnipeg time')
        })

        // An engine may refuse with an error type that is not a RangeError; the
        // header still has a line to show.
        it('reads an unknown zone as UTC when the engine throws a plain Error', () => {
            function Failing(): Intl.DateTimeFormat {
                throw new Error('engine failure')
            }
            spyOnDateTimeFormat(Failing)
            expect(timeZoneWords('America/Edmonton')).toBe('UTC')
        })

        // zoneName's catch takes any error type, not only a RangeError.
        it('falls back to the long name when the generic style throws a plain Error', () => {
            function FailingGeneric(
                locale?: Intl.LocalesArgument,
                options?: Intl.DateTimeFormatOptions
            ) {
                if (options?.timeZoneName === 'longGeneric') throw new Error('engine failure')
                return new RealDateTimeFormat(locale, options)
            }
            spyOnDateTimeFormat(FailingGeneric)
            expect(timeZoneWords('Asia/Tokyo')).toBe('Japan Time')
        })

        it('builds its formatters once per zone', () => {
            const spy = spyOnZoneNames(() => undefined)
            expect(timeZoneWords('Europe/Paris')).toBe('Central European Time')
            const built = spy.mock.calls.length
            expect(timeZoneWords('Europe/Paris')).toBe('Central European Time')
            expect(spy.mock.calls.length).toBe(built)
        })
    })

    it("defaults to the reader's zone", () => {
        expect(timeZoneWords()).toBe(timeZoneWords(readerTimeZone()))
    })
})
