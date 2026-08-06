import { describe, expect, it } from 'vitest'
import { formatLastUsed } from '../../components/settings/ConnectedAppsSection'

describe('formatLastUsed', () => {
    it('reports never for an empty timestamp', () => {
        expect(formatLastUsed('')).toBe('Never used')
    })

    it('reports a relative time for a recent timestamp', () => {
        const oneHourAgo = new Date(Date.now() - 60 * 60 * 1000).toISOString()
        expect(formatLastUsed(oneHourAgo)).toContain('hour')
    })

    it('reports days for an older timestamp', () => {
        const threeDaysAgo = new Date(Date.now() - 3 * 24 * 60 * 60 * 1000).toISOString()
        expect(formatLastUsed(threeDaysAgo)).toContain('day')
    })
})
