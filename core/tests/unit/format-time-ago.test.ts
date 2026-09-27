import { describe, expect, it } from 'vitest'
import { formatTimeAgo } from '../../lib/format-utils'

const now = new Date('2026-09-24T12:00:00Z')

describe('formatTimeAgo', () => {
    it('handles PocketBase space-separated timestamps', () => {
        expect(formatTimeAgo('2026-09-24 11:59:40.000Z', now)).toBe('just now')
    })
    it('minutes, hours, days', () => {
        expect(formatTimeAgo('2026-09-24T11:55:00Z', now)).toBe('5m ago')
        expect(formatTimeAgo('2026-09-24T09:00:00Z', now)).toBe('3h ago')
        expect(formatTimeAgo('2026-09-22T12:00:00Z', now)).toBe('2d ago')
    })
    it('falls back to a date beyond a week', () => {
        expect(formatTimeAgo('2026-09-01T12:00:00Z', now)).toBe(
            new Date('2026-09-01T12:00:00Z').toLocaleDateString()
        )
    })
})
