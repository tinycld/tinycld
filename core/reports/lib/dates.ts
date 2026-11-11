import { toDateString } from '@tinycld/core/lib/dates'
import type { BucketLabelType } from './types'

// Report days are YYYY-MM-DD. Month names are local, so the words do not depend
// on the runtime's locale data (Hermes ships little of it).

const FIRST_YEAR = 1900
const MONTH_NAMES = [
    'January',
    'February',
    'March',
    'April',
    'May',
    'June',
    'July',
    'August',
    'September',
    'October',
    'November',
    'December',
]
const SHORT_MONTH_NAMES = MONTH_NAMES.map(name => name.slice(0, 3))

function daysInMonth(year: number, month: number): number {
    return new Date(Date.UTC(year, month, 0)).getUTCDate()
}

/** The engine's day rule: YYYY-MM-DD, a real day, year 1900 or later. */
export function isIsoDay(text: string): boolean {
    if (!/^\d{4}-\d{2}-\d{2}$/.test(text)) return false
    const year = Number(text.slice(0, 4))
    const month = Number(text.slice(5, 7))
    const day = Number(text.slice(8, 10))
    if (year < FIRST_YEAR || month < 1 || month > 12 || day < 1) return false
    return day <= daysInMonth(year, month)
}

/** The reader's own day, which period presets resolve against. */
export function todayIso(): string {
    return toDateString(new Date())
}

function monthDay(day: string, names: readonly string[]): string {
    return `${names[Number(day.slice(5, 7)) - 1]} ${Number(day.slice(8, 10))}`
}

export function dayWords(day: string): string {
    return `${monthDay(day, MONTH_NAMES)}, ${day.slice(0, 4)}`
}

export function shortDay(day: string): string {
    return `${monthDay(day, SHORT_MONTH_NAMES)}, ${day.slice(0, 4)}`
}

const MONTH_KEY = /^(\d{4})-(\d{2})$/
const QUARTER_KEY = /^(\d{4})-Q([1-4])$/
const YEAR_KEY = /^\d{4}$/

/**
 * A bucket key as the view shows it: month "Jan 2026", quarter "Q1 2026", year
 * "2026", fiscal year "FY 2026" (the year it ends in), day "Jan 5, 2026". A key
 * not in its bucket's form shows as it is.
 */
export function bucketWords(type: BucketLabelType, key: string | number): string {
    const text = String(key)
    switch (type) {
        case 'month': {
            const match = MONTH_KEY.exec(text)
            const name = match ? SHORT_MONTH_NAMES[Number(match[2]) - 1] : undefined
            return match && name ? `${name} ${match[1]}` : text
        }
        case 'quarter': {
            const match = QUARTER_KEY.exec(text)
            return match ? `Q${match[2]} ${match[1]}` : text
        }
        case 'fiscalYear':
            return YEAR_KEY.test(text) ? `FY ${text}` : text
        case 'day':
            return isIsoDay(text) ? shortDay(text) : text
        case 'year':
            return text
    }
}

// One day for a one-day range; the first year is dropped when both ends share it.
function rangeText(
    from: string,
    to: string,
    withYear: (day: string) => string,
    withoutYear: (day: string) => string
): string {
    if (from === to) return withYear(to)
    const start = from.slice(0, 4) === to.slice(0, 4) ? withoutYear(from) : withYear(from)
    return `${start} – ${withYear(to)}`
}

export function rangeWords(from: string, to: string): string {
    return rangeText(from, to, dayWords, day => monthDay(day, MONTH_NAMES))
}

export function rangeChipText(from: string, to: string): string {
    return rangeText(from, to, shortDay, day => monthDay(day, SHORT_MONTH_NAMES))
}
