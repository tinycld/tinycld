import { describe, expect, it } from 'vitest'
import {
    addDays,
    addMonths,
    endOfMonth,
    fromDateString,
    getMonthGrid,
    isSameDay,
    startOfMonth,
    startOfWeek,
    toDateString,
} from '../../lib/dates'

// These helpers are day-granular and LOCAL-TIME. The cases below are the ones
// hand-rolled date math actually gets wrong: month-length clamping, DST
// transitions, and the UTC round-trip that shifts a date by one day.

describe('addDays', () => {
    it('crosses a month boundary', () => {
        expect(toDateString(addDays(new Date(2026, 0, 31), 1))).toBe('2026-02-01')
    })

    it('crosses a year boundary', () => {
        expect(toDateString(addDays(new Date(2026, 11, 31), 1))).toBe('2027-01-01')
    })

    it('handles leap day', () => {
        expect(toDateString(addDays(new Date(2028, 1, 28), 1))).toBe('2028-02-29')
        expect(toDateString(addDays(new Date(2026, 1, 28), 1))).toBe('2026-03-01')
    })

    it('goes backwards', () => {
        expect(toDateString(addDays(new Date(2026, 2, 1), -1))).toBe('2026-02-28')
    })

    it('does not mutate its argument', () => {
        const original = new Date(2026, 5, 15)
        addDays(original, 10)
        expect(toDateString(original)).toBe('2026-06-15')
    })

    // In US timezones 2026-03-08 is the spring-forward day (23 hours long).
    // Adding a day by hours would land mid-afternoon on the same date; the
    // setDate-based implementation steps the calendar day instead.
    it('steps one calendar day across a DST transition', () => {
        expect(toDateString(addDays(new Date(2026, 2, 7), 1))).toBe('2026-03-08')
        expect(toDateString(addDays(new Date(2026, 2, 8), 1))).toBe('2026-03-09')
    })
})

describe('addMonths', () => {
    it('clamps to the last day of a shorter month', () => {
        // The bug this exists for: setMonth alone turns Jan 31 into Mar 3.
        expect(toDateString(addMonths(new Date(2026, 0, 31), 1))).toBe('2026-02-28')
        expect(toDateString(addMonths(new Date(2028, 0, 31), 1))).toBe('2028-02-29')
        expect(toDateString(addMonths(new Date(2026, 4, 31), 1))).toBe('2026-06-30')
    })

    it('keeps the day when the target month is long enough', () => {
        expect(toDateString(addMonths(new Date(2026, 0, 15), 1))).toBe('2026-02-15')
    })

    it('crosses a year boundary in both directions', () => {
        expect(toDateString(addMonths(new Date(2026, 11, 15), 1))).toBe('2027-01-15')
        expect(toDateString(addMonths(new Date(2026, 0, 15), -1))).toBe('2025-12-15')
    })

    it('does not mutate its argument', () => {
        const original = new Date(2026, 0, 31)
        addMonths(original, 1)
        expect(toDateString(original)).toBe('2026-01-31')
    })
})

describe('startOfWeek', () => {
    it('returns the Sunday of the week by default', () => {
        // 2026-06-17 is a Wednesday.
        expect(toDateString(startOfWeek(new Date(2026, 5, 17)))).toBe('2026-06-14')
    })

    it('honours a Monday week start', () => {
        expect(toDateString(startOfWeek(new Date(2026, 5, 17), 1))).toBe('2026-06-15')
    })

    it('is a no-op on the first day of the week', () => {
        expect(toDateString(startOfWeek(new Date(2026, 5, 14)))).toBe('2026-06-14')
    })

    it('zeroes the time', () => {
        const result = startOfWeek(new Date(2026, 5, 17, 23, 59, 59))
        expect([result.getHours(), result.getMinutes(), result.getSeconds()]).toEqual([0, 0, 0])
    })
})

describe('startOfMonth / endOfMonth', () => {
    it('finds both ends of a month', () => {
        expect(toDateString(startOfMonth(new Date(2026, 5, 17)))).toBe('2026-06-01')
        expect(toDateString(endOfMonth(new Date(2026, 5, 17)))).toBe('2026-06-30')
    })

    it('gets February right in leap and common years', () => {
        expect(toDateString(endOfMonth(new Date(2026, 1, 10)))).toBe('2026-02-28')
        expect(toDateString(endOfMonth(new Date(2028, 1, 10)))).toBe('2028-02-29')
    })
})

describe('toDateString', () => {
    it('pads month and day', () => {
        expect(toDateString(new Date(2026, 0, 5))).toBe('2026-01-05')
    })

    // The regression this function exists for: toISOString() converts to UTC,
    // so a late-evening local time stamps as the NEXT day in any timezone west
    // of Greenwich. Formatting from the local getters cannot drift.
    it('uses the local calendar day, not the UTC one', () => {
        const lateEvening = new Date(2026, 2, 14, 23, 30)
        expect(toDateString(lateEvening)).toBe('2026-03-14')
        const earlyMorning = new Date(2026, 2, 14, 0, 30)
        expect(toDateString(earlyMorning)).toBe('2026-03-14')
    })
})

describe('fromDateString', () => {
    it('round-trips with toDateString', () => {
        const parsed = fromDateString('2026-03-14')
        expect(parsed && toDateString(parsed)).toBe('2026-03-14')
    })

    it('parses as a local day rather than UTC midnight', () => {
        // new Date('2026-03-14') is UTC midnight — the previous day in the US.
        const parsed = fromDateString('2026-03-14')
        expect(parsed?.getDate()).toBe(14)
        expect(parsed?.getMonth()).toBe(2)
        expect(parsed?.getHours()).toBe(0)
    })

    it('rejects malformed input', () => {
        for (const bad of ['', 'tomorrow', '2026-3-14', '26-03-14', '2026/03/14', 'x2026-03-14']) {
            expect(fromDateString(bad)).toBeNull()
        }
    })

    it('rejects dates that do not exist', () => {
        // The Date constructor rolls these over silently; the caller must not
        // get a valid-looking date back from an impossible string.
        expect(fromDateString('2026-02-31')).toBeNull()
        expect(fromDateString('2026-13-01')).toBeNull()
        expect(fromDateString('2026-00-10')).toBeNull()
    })

    it('accepts a real leap day and rejects a fake one', () => {
        expect(fromDateString('2028-02-29')).not.toBeNull()
        expect(fromDateString('2026-02-29')).toBeNull()
    })

    it('tolerates surrounding whitespace', () => {
        expect(fromDateString('  2026-03-14 ')).not.toBeNull()
    })
})

describe('getMonthGrid', () => {
    it('always returns 42 cells so the grid height never changes', () => {
        // February 2026 starts on a Sunday and fits in exactly 4 weeks — the
        // month most likely to collapse a naive grid.
        for (const month of [0, 1, 5, 11]) {
            expect(getMonthGrid(new Date(2026, month, 1))).toHaveLength(42)
        }
    })

    it('starts on the week containing the first of the month', () => {
        // 2026-06-01 is a Monday, so a Sunday-start grid opens on May 31.
        const grid = getMonthGrid(new Date(2026, 5, 1))
        expect(toDateString(grid[0].date)).toBe('2026-05-31')
        expect(grid[0].isCurrentMonth).toBe(false)
    })

    it('runs consecutive days with no gaps or repeats', () => {
        const grid = getMonthGrid(new Date(2026, 5, 1))
        for (let i = 1; i < grid.length; i += 1) {
            expect(isSameDay(grid[i].date, addDays(grid[i - 1].date, 1))).toBe(true)
        }
    })

    it('marks in-month cells', () => {
        const grid = getMonthGrid(new Date(2026, 5, 1))
        expect(grid.filter(cell => cell.isCurrentMonth)).toHaveLength(30)
    })

    it('honours a Monday week start', () => {
        const grid = getMonthGrid(new Date(2026, 5, 1), 1)
        expect(toDateString(grid[0].date)).toBe('2026-06-01')
    })

    it('marks exactly one cell as today when the month is the current one', () => {
        const now = new Date()
        const grid = getMonthGrid(now)
        expect(grid.filter(cell => cell.isToday && cell.isCurrentMonth)).toHaveLength(1)
    })
})
