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

describe('formatTarget', () => {
    it('sorts by package', () => {
        expect(formatTarget({ mail: '0.6.0', core: '0.5.4' })).toBe('core 0.5.4, mail 0.6.0')
    })
})
