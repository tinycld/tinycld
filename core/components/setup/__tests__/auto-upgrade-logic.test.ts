import { describe, expect, it } from 'vitest'
import { formatTarget, isOn, statusLine, windowSchema } from '../auto-upgrade-logic'

describe('isOn', () => {
    it('is on only for the stored string "true"', () => {
        expect(isOn('true')).toBe(true)
        expect(isOn('false')).toBe(false)
        expect(isOn(undefined)).toBe(false)
    })
})

describe('windowSchema', () => {
    it('accepts HH:MM-HH:MM and refuses equal ends', () => {
        expect(windowSchema.safeParse('02:00-05:00').success).toBe(true)
        expect(windowSchema.safeParse('23:00-01:00').success).toBe(true)
        expect(windowSchema.safeParse('2:00-5:00').success).toBe(false)
        expect(windowSchema.safeParse('02:00-02:00').success).toBe(false)
        expect(windowSchema.safeParse('24:00-01:00').success).toBe(false)
    })
    it('refuses a window shorter than 60 minutes, also across midnight', () => {
        expect(windowSchema.safeParse('02:00-03:00').success).toBe(true)
        expect(windowSchema.safeParse('23:30-00:30').success).toBe(true)
        const short = windowSchema.safeParse('02:00-02:30')
        expect(short.success).toBe(false)
        expect(short.error?.issues[0]?.message).toBe('The window must be at least 60 minutes long')
        expect(windowSchema.safeParse('23:45-00:15').success).toBe(false)
    })
})

describe('statusLine', () => {
    it('shows the reason when unavailable', () => {
        expect(
            statusLine({
                available: false,
                reason: 'No toolchain',
                lastRun: '',
                lastResult: '',
                nextCheck: '',
            })
        ).toBe('No toolchain')
    })
    it('shows the last result and next check', () => {
        expect(
            statusLine({
                available: true,
                lastRun: '2026-10-01T03:00:00Z',
                lastResult: 'no updates',
                nextCheck: '2026-10-02T02:00:00Z',
            })
        ).toMatch(/^Last check: no updates · Next check: /)
    })
    it('says never when there is no run yet', () => {
        expect(
            statusLine({
                available: true,
                lastRun: '0001-01-01T00:00:00Z',
                lastResult: '',
                nextCheck: '2026-10-02T02:00:00Z',
            })
        ).toMatch(/^No check yet · Next check: /)
    })
})

describe('statusLine without a next check', () => {
    const zero = '0001-01-01T00:00:00Z'
    it('says the switch is off instead of a next check', () => {
        const line = statusLine({
            available: true,
            lastRun: zero,
            lastResult: 'off',
            nextCheck: zero,
        })
        expect(line).toBe('Automatic updates are off.')
    })
    it('shows why checks are disabled instead of a next check', () => {
        const line = statusLine({
            available: true,
            lastRun: zero,
            lastResult: 'checks disabled: development build',
            nextCheck: zero,
        })
        expect(line).toBe('Checks disabled: development build')
    })
})

describe('formatTarget', () => {
    it('sorts by package', () => {
        expect(formatTarget({ mail: '0.6.0', core: '0.5.4' })).toBe('core 0.5.4, mail 0.6.0')
    })
})
