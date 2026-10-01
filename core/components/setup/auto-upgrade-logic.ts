import { z } from '@tinycld/core/ui/form'

// Pure, React/pbtsdb-free rules behind the automatic-updates controls, so they
// are unit-testable without rendering.

export const KEY_ENABLED = 'autoupgrade.enabled'
export const KEY_WINDOW = 'autoupgrade.window'
export const DEFAULT_WINDOW = '02:00-05:00'

export interface AutoUpgradeStatus {
    available: boolean
    reason?: string
    lastRun: string
    lastResult: string
    nextCheck: string
}

export interface AutoUpgradeStatusResponse {
    windowManaged: boolean
    status: AutoUpgradeStatus
}

const WINDOW_PATTERN = /^([01]\d|2[0-3]):[0-5]\d-([01]\d|2[0-3]):[0-5]\d$/

const MIN_WINDOW_MINUTES = 60
const DAY_MINUTES = 24 * 60

function clockMinutes(hhmm: string): number {
    const [h, m] = hhmm.split(':').map(Number)
    return h * 60 + m
}

// Matches the server's autoupgrade.Window.Length: an end before the start
// means the window crosses midnight.
function windowMinutes(value: string): number {
    const start = clockMinutes(value.slice(0, 5))
    const end = clockMinutes(value.slice(6))
    return end > start ? end - start : DAY_MINUTES - start + end
}

function isLongEnough(value: string): boolean {
    if (!WINDOW_PATTERN.test(value)) return true // the pattern rule reports it
    return windowMinutes(value) >= MIN_WINDOW_MINUTES
}

export const windowSchema = z
    .string()
    .regex(WINDOW_PATTERN, 'Use HH:MM-HH:MM, for example 02:00-05:00')
    .refine(v => v.slice(0, 5) !== v.slice(6), 'Start and end must differ')
    .refine(isLongEnough, 'The window must be at least 60 minutes long')

export function isOn(value: string | undefined): boolean {
    return value === 'true'
}

// Go marshals a zero time.Time as year 1; treat it as "never".
function hasRun(iso: string): boolean {
    return iso !== '' && !iso.startsWith('0001-')
}

function shortDate(iso: string): string {
    return new Date(iso).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' })
}

export function statusLine(s: AutoUpgradeStatus): string {
    if (!s.available) return s.reason ?? 'Automatic updates are not available on this server.'
    const last = hasRun(s.lastRun) ? `Last check: ${s.lastResult}` : 'No check yet'
    return `${last} · Next check: ${shortDate(s.nextCheck)}`
}

export function formatTarget(target: Record<string, string>): string {
    return Object.keys(target)
        .sort()
        .map(slug => `${slug} ${target[slug]}`)
        .join(', ')
}
