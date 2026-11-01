import { describe, expect, it } from 'vitest'
import { recentKey, withRun } from '../report-recent-store'

describe('withRun', () => {
    it('records a report’s last run for one reader', () => {
        expect(withRun({}, recentKey('u1', 'r1'), '2026-10-31T10:00:00.000Z')).toEqual({
            'u1:r1': '2026-10-31T10:00:00.000Z',
        })
    })

    it('keeps the newest runs when the list is full', () => {
        const lastRun = {
            'u1:a': '2026-10-01T00:00:00.000Z',
            'u1:b': '2026-10-03T00:00:00.000Z',
            'u1:c': '2026-10-02T00:00:00.000Z',
        }
        expect(withRun(lastRun, 'u1:d', '2026-10-04T00:00:00.000Z', 3)).toEqual({
            'u1:b': '2026-10-03T00:00:00.000Z',
            'u1:c': '2026-10-02T00:00:00.000Z',
            'u1:d': '2026-10-04T00:00:00.000Z',
        })
    })
})
