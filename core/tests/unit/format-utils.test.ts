import { formatBytes, formatDate } from '@tinycld/core/lib/format-utils'
import { describe, expect, it } from 'vitest'

describe('formatDate', () => {
    it('formats a valid ISO date', () => {
        // Use a UTC-noon timestamp so the local-date result is timezone-stable.
        expect(formatDate('2026-03-14T12:00:00Z')).toMatch(/Mar 14, 2026/)
    })

    // The "Invalid Date" bug: an absent date (a drive search row before its
    // record loads carries no `updated`) must render as '' rather than the
    // literal "Invalid Date". This is the only invalid input our own callers
    // produce — the API otherwise always emits well-formed dates.
    it('returns empty string for an absent date', () => {
        expect(formatDate('')).toBe('')
    })
})

describe('formatBytes', () => {
    it('renders an em dash for zero', () => {
        expect(formatBytes(0)).toBe('—')
    })

    it('formats bytes/KB/MB/GB', () => {
        expect(formatBytes(512)).toBe('512 B')
        expect(formatBytes(1024)).toBe('1.0 KB')
        expect(formatBytes(1024 * 1024)).toBe('1.0 MB')
        expect(formatBytes(1024 * 1024 * 1024)).toBe('1.0 GB')
    })
})
