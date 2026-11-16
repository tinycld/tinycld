import { isIsoDay, shortDay } from './dates'

// A date-time value is an instant as PocketBase stores it ("2026-11-14 22:07:00.000Z").
// It shows as the day and the time on the reader's wall clock. Intl gives only the
// numbers; the words are core's own, as the day words are (dates.ts), because
// Hermes ships little locale data (R4).

const INSTANT = /^\d{4}-\d{2}-\d{2}[ T]\d{2}:\d{2}(:\d{2}(\.\d{1,3})?)?Z$/
// The names of UTC itself. The server's readerZone (zone.go) reads the same set as
// UTC, so the two never disagree on which reader is in UTC.
const UTC_ZONES: ReadonlySet<string> = new Set(['UTC', 'Etc/UTC', 'Etc/GMT', 'GMT'])
const WALL_CLOCK: Intl.DateTimeFormatOptions = {
    year: 'numeric',
    month: 'numeric',
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
    hourCycle: 'h23',
}
// Any day serves: a zone's generic name does not change with the season.
const REFERENCE_DAY = new Date(Date.UTC(2026, 0, 15))

type DeviceClock = { timeZone: string; hour12: boolean }
let deviceClock: DeviceClock | null = null

/**
 * Whether the device locale's clock is 12-hour, from Intl's resolved options. An
 * engine that leaves out hourCycle (some Hermes builds) may still give hour12; a
 * locale that gives neither reads 12-hour (R4).
 */
export function isTwelveHour(
    resolved: Pick<Intl.ResolvedDateTimeFormatOptions, 'hourCycle' | 'hour12'>
): boolean {
    if (resolved.hourCycle) return resolved.hourCycle === 'h11' || resolved.hourCycle === 'h12'
    if (resolved.hour12 !== undefined) return resolved.hour12
    return true
}

// Read once per app start (R3), so a cell, the header's zone line and the run
// request always name one zone.
function readDeviceClock(): DeviceClock {
    if (deviceClock) return deviceClock
    const timeZone = new Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC'
    const resolved = new Intl.DateTimeFormat(undefined, { hour: 'numeric' }).resolvedOptions()
    deviceClock = { timeZone, hour12: isTwelveHour(resolved) }
    return deviceClock
}

/** The reader's IANA time zone, as the run request sends it. */
export function readerTimeZone(): string {
    return readDeviceClock().timeZone
}

/** Whether the reader's clock is 12-hour, as the view's date-time cells show it. */
export function readerHour12(): boolean {
    return readDeviceClock().hour12
}

function isKnownZone(timeZone: string): boolean {
    try {
        new Intl.DateTimeFormat('en-US', { timeZone })
        return true
    } catch {
        // Whatever an engine throws for a name it cannot use, the zone reads as unknown.
        return false
    }
}

const formatters = new Map<string, Intl.DateTimeFormat>()

// A zone Intl does not know reads as UTC, as the server reads it (R2), so the
// cells and the rows a filter keeps agree. The cache is keyed by the zone used, so
// unknown names never add entries.
function wallClockFormat(timeZone: string): Intl.DateTimeFormat {
    const known = formatters.get(timeZone)
    if (known) return known
    const zone = isKnownZone(timeZone) ? timeZone : 'UTC'
    const cached = formatters.get(zone)
    if (cached) return cached
    const made = new Intl.DateTimeFormat('en-US', { ...WALL_CLOCK, timeZone: zone })
    formatters.set(zone, made)
    return made
}

const pad = (n: number) => String(n).padStart(2, '0')

function partOf(parts: readonly Intl.DateTimeFormatPart[], type: Intl.DateTimeFormatPartTypes) {
    return parts.find(part => part.type === type)?.value ?? ''
}

// The hour on a 24-hour clock. An engine that ignores hourCycle answers with a
// day period (AM/PM), which is read too.
export function hour24(parts: readonly Intl.DateTimeFormatPart[]): number {
    const hour = Number(partOf(parts, 'hour'))
    const period = partOf(parts, 'dayPeriod')
    if (!period) return hour % 24
    return (hour % 12) + (/^p/i.test(period) ? 12 : 0)
}

function clockText(hour: number, minute: string, hour12: boolean): string {
    if (!hour12) return `${pad(hour)}:${minute}`
    return `${hour % 12 || 12}:${minute} ${hour < 12 ? 'AM' : 'PM'}`
}

function instantOf(value: string): Date | null {
    if (!INSTANT.test(value)) return null
    const date = new Date(value.replace(' ', 'T'))
    return Number.isNaN(date.getTime()) ? null : date
}

/**
 * An instant as "Nov 14, 2026, 4:07 PM" (or "Nov 14, 2026, 16:07") on the zone's
 * wall clock. A bare day shows as the short day; any other text shows as it is.
 */
export function formatDateTime(value: string, timeZone: string, hour12: boolean): string {
    if (isIsoDay(value)) return shortDay(value)
    const instant = instantOf(value)
    if (!instant) return value
    const parts = wallClockFormat(timeZone).formatToParts(instant)
    const month = pad(Number(partOf(parts, 'month')))
    const day = pad(Number(partOf(parts, 'day')))
    const date = `${partOf(parts, 'year')}-${month}-${day}`
    return `${shortDay(date)}, ${clockText(hour24(parts), partOf(parts, 'minute'), hour12)}`
}

/** A date-time as a report cell shows it: in the reader's zone and hour cycle. */
export function dateTimeText(value: string, timeZone?: string): string {
    const clock = readDeviceClock()
    return formatDateTime(value, timeZone ?? clock.timeZone, clock.hour12)
}

function zoneName(timeZone: string, style: 'long' | 'longGeneric'): string | null {
    try {
        const parts = new Intl.DateTimeFormat('en-US', {
            timeZone,
            timeZoneName: style,
        }).formatToParts(REFERENCE_DAY)
        const name = partOf(parts, 'timeZoneName')
        // A short name ("CST") or a GMT offset is not words; an engine that ignores
        // the style may answer with one.
        return name.includes(' ') && !name.startsWith('GMT') ? name : null
    } catch {
        // An engine without the generic style refuses it, with whatever error type it
        // uses; this runs in render, where a throw would crash the report.
        return null
    }
}

const zoneWords = new Map<string, string>()

/**
 * A zone in words for the header's line (R6): "Central Time"; "UTC" for UTC and
 * for a zone Intl does not know; else the zone's city ("Chicago time"). Cached per
 * zone: the header asks on every render, and building formatters is slow on Hermes.
 */
export function timeZoneWords(timeZone = readerTimeZone()): string {
    const known = zoneWords.get(timeZone)
    if (known !== undefined) return known
    const words = wordsOfZone(timeZone)
    zoneWords.set(timeZone, words)
    return words
}

function wordsOfZone(timeZone: string): string {
    if (UTC_ZONES.has(timeZone) || !isKnownZone(timeZone)) return 'UTC'
    const generic = zoneName(timeZone, 'longGeneric')
    if (generic) return generic
    const long = zoneName(timeZone, 'long')
    if (long) return long.replace(' Standard ', ' ').replace(' Daylight ', ' ')
    const city = timeZone.split('/').pop() ?? timeZone
    return `${city.split('_').join(' ')} time`
}
