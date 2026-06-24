import { describe, expect, it, vi } from 'vitest'
import { findSentinelBundleId, queryA11ySentinel } from '../a11y-sentinel'

// idb ui describe-all --json emits a flat array of element objects. Field keys
// vary by idb version, so the parser tolerates AXIdentifier/AXLabel and the
// normalized identifier/label spellings.
const treeAX = [
    { AXIdentifier: 'some-button', AXLabel: 'Tap me' },
    { AXIdentifier: 'ota-bundle-sentinel', AXLabel: 'bundle:build-55-ios' },
]
const treeNormalized = [{ identifier: 'ota-bundle-sentinel', label: 'bundle:build-77-ios' }]

describe('findSentinelBundleId', () => {
    it('finds the sentinel by AXIdentifier and parses bundle:<id>', () => {
        expect(findSentinelBundleId(treeAX)).toBe('build-55-ios')
    })
    it('tolerates the normalized identifier/label spelling', () => {
        expect(findSentinelBundleId(treeNormalized)).toBe('build-77-ios')
    })
    it('returns null when no sentinel element is present', () => {
        expect(findSentinelBundleId([{ AXIdentifier: 'x', AXLabel: 'y' }])).toBeNull()
    })
    it('returns null on a non-array (defensive)', () => {
        expect(findSentinelBundleId({})).toBeNull()
        expect(findSentinelBundleId(null)).toBeNull()
    })
})

describe('queryA11ySentinel', () => {
    it('returns the sentinel id from the idb runner output', async () => {
        const runner = vi.fn(() => Promise.resolve(JSON.stringify(treeAX)))
        expect(await queryA11ySentinel('UDID-1', runner)).toBe('build-55-ios')
    })
    it('returns null (skip) when the runner reports idb is unavailable', async () => {
        const runner = vi.fn(() => Promise.resolve(null))
        expect(await queryA11ySentinel('UDID-1', runner)).toBeNull()
    })
    it('returns null when the output is not valid JSON', async () => {
        const runner = vi.fn(() => Promise.resolve('not json'))
        expect(await queryA11ySentinel('UDID-1', runner)).toBeNull()
    })
})
