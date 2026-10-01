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

export const windowSchema = z
    .string()
    .regex(WINDOW_PATTERN, 'Use HH:MM-HH:MM, for example 02:00-05:00')
    .refine(v => v.slice(0, 5) !== v.slice(6), 'Start and end must differ')

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
