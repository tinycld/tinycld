import { bootLogLine, formatSentinelLabel, shortHash } from '@tinycld/core/lib/bundle-sentinel'
import { describe, expect, it } from 'vitest'

describe('shortHash', () => {
    it('takes the first 12 chars', () => {
        expect(shortHash('abcdef0123456789aaaa')).toBe('abcdef012345')
    })
    it('returns empty for an empty hash', () => {
        expect(shortHash('')).toBe('')
    })
})

describe('formatSentinelLabel', () => {
    it('prefixes the bundle id with bundle:', () => {
        expect(formatSentinelLabel('build-123-ios')).toBe('bundle:build-123-ios')
    })
})

describe('bootLogLine', () => {
    it('emits the stable, scrapeable boot line with id and short hash', () => {
        expect(bootLogLine('build-123-ios', 'abcdef0123456789')).toBe(
            '[tinycld] app-boot: rendered bundle id=build-123-ios hash=abcdef012345'
        )
    })
})
