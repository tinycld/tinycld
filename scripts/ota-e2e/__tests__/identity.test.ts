import { describe, expect, it } from 'vitest'
import { classifyBundleId, embeddedIdForVersion } from '../identity'

describe('embeddedIdForVersion', () => {
    it('formats the embedded id from an app version', () => {
        expect(embeddedIdForVersion('1.13.7')).toBe('embedded-1.13.7')
    })
})

describe('classifyBundleId', () => {
    it('recognizes an embedded id', () => {
        expect(classifyBundleId('embedded-1.13.7')).toBe('embedded')
    })
    it('recognizes a server build id', () => {
        expect(classifyBundleId('build-1718200000000-ios')).toBe('server')
    })
    it('returns unknown for anything else', () => {
        expect(classifyBundleId('')).toBe('unknown')
        expect(classifyBundleId('garbage')).toBe('unknown')
    })
})
