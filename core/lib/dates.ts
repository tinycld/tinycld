// Calendar-day arithmetic, in local time.
//
// Promoted here from the calendar package (its `useCalendarNavigation` /
// `useMonthGrid`), which was the only place in the workspace that could build a
// month grid — so boards could not offer a due-date picker without either
// duplicating it or importing a sibling, and siblings must not depend on each
// other.
//
// EVERY FUNCTION HERE IS LOCAL-TIME AND DAY-GRANULAR. That is the whole
// contract: a due date, a month grid cell and a "which day is this" comparison
// are calendar-day concepts, and converting through UTC is what makes a date
// render as the previous day for anyone west of Greenwich. `new Date(y, m, d)`
// and the `setDate`/`setMonth` mutators used below all operate in local time and
// normalize overflow (Jan 31 + 1 month, Dec 31 + 1 day) for us.

/** A copy of `date` shifted by whole days. */
export function addDays(date: Date, days: number): Date {
    const result = new Date(date)
    result.setDate(result.getDate() + days)
    return result
}

/** A copy of `date` shifted by whole weeks. */
export function addWeeks(date: Date, weeks: number): Date {
    return addDays(date, weeks * 7)
}

/**
 * A copy of `date` shifted by whole months.
 *
 * Clamps rather than overflows the way `setMonth` alone would: Jan 31 + 1 month
 * is Feb 28 (or 29), not Mar 3. Month-stepping a calendar grid from a 31st is
 * exactly how the naive version surfaces.
 */
export function addMonths(date: Date, months: number): Date {
    const result = new Date(date)
    const targetDay = result.getDate()
    result.setDate(1)
    result.setMonth(result.getMonth() + months)
    const lastDay = endOfMonth(result).getDate()
    result.setDate(Math.min(targetDay, lastDay))
    return result
}

/** Midnight on the first day of `date`'s week. `startDay` 0 = Sunday. */
export function startOfWeek(date: Date, startDay = 0): Date {
    const result = new Date(date)
    result.setHours(0, 0, 0, 0)
    const day = result.getDay()
    const diff = (day - startDay + 7) % 7
    result.setDate(result.getDate() - diff)
    return result
}

/** Midnight on the first of `date`'s month. */
export function startOfMonth(date: Date): Date {
    return new Date(date.getFullYear(), date.getMonth(), 1)
}

/** The last day of `date`'s month — day 0 of the next one. */
export function endOfMonth(date: Date): Date {
    return new Date(date.getFullYear(), date.getMonth() + 1, 0)
}

/** Midnight at the start of `date`'s day. */
export function startOfDay(date: Date): Date {
    const result = new Date(date)
    result.setHours(0, 0, 0, 0)
    return result
}

export function isSameDay(a: Date, b: Date): boolean {
    return (
        a.getFullYear() === b.getFullYear() &&
        a.getMonth() === b.getMonth() &&
        a.getDate() === b.getDate()
    )
}

export function isToday(date: Date): boolean {
    return isSameDay(date, new Date())
}

export function getDaysInMonth(date: Date): number {
    return endOfMonth(date).getDate()
}

/**
 * `YYYY-MM-DD` in LOCAL time.
 *
 * Deliberately not `toISOString().slice(0, 10)`, which converts to UTC first
 * and therefore returns the previous day for any local time before the UTC
 * offset — an evening in New York stamps as tomorrow's date in Europe.
 */
export function toDateString(date: Date): string {
    const month = String(date.getMonth() + 1).padStart(2, '0')
    const day = String(date.getDate()).padStart(2, '0')
    return `${date.getFullYear()}-${month}-${day}`
}

/**
 * Parse `YYYY-MM-DD` as a LOCAL calendar day, or null if malformed.
 *
 * `new Date('2026-03-14')` parses as UTC midnight per the ECMAScript spec, so
 * it renders as March 13 anywhere west of Greenwich. Splitting the parts and
 * handing them to the local-time constructor is what avoids that.
 */
export function fromDateString(value: string): Date | null {
    const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value.trim())
    if (!match) return null

    const year = Number(match[1])
    const month = Number(match[2])
    const day = Number(match[3])
    const parsed = new Date(year, month - 1, day)

    // Rejects real-looking impossibilities (2026-02-31, month 13): the Date
    // constructor rolls those over silently rather than failing.
    if (
        parsed.getFullYear() !== year ||
        parsed.getMonth() !== month - 1 ||
        parsed.getDate() !== day
    ) {
        return null
    }
    return parsed
}

export interface MonthGridCell {
    date: Date
    isCurrentMonth: boolean
    isToday: boolean
}

/**
 * The 6×7 cell grid a month view renders.
 *
 * Always 42 cells, even when the month fits in five rows: a grid that changes
 * height as you page through months makes the surrounding layout jump.
 */
export function getMonthGrid(monthDate: Date, weekStartDay = 0): MonthGridCell[] {
    const monthStart = startOfMonth(monthDate)
    const gridStart = startOfWeek(monthStart, weekStartDay)
    const currentMonth = monthDate.getMonth()

    return Array.from({ length: 42 }, (_, i) => {
        const date = addDays(gridStart, i)
        return {
            date,
            isCurrentMonth: date.getMonth() === currentMonth,
            isToday: isToday(date),
        }
    })
}
