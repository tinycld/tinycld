import { describe, expect, it } from 'vitest'
import { extractBootBeaconIds } from '../boot-beacon-poller'

describe('extractBootBeaconIds', () => {
    it('pulls q.bundleId from each log item, in order', () => {
        const ids = extractBootBeaconIds({
            items: [
                { data: { 'q.bundleId': 'build-9-ios' } },
                { data: { 'q.bundleId': 'embedded-2.0.0' } },
            ],
        })
        expect(ids).toEqual(['build-9-ios', 'embedded-2.0.0'])
    })

    it('skips items missing data or the bundleId field', () => {
        const ids = extractBootBeaconIds({
            items: [{ data: {} }, {}, { data: { 'q.bundleId': 'build-1-ios' } }],
        })
        expect(ids).toEqual(['build-1-ios'])
    })

    it('tolerates a missing items array', () => {
        expect(extractBootBeaconIds({})).toEqual([])
    })
})
