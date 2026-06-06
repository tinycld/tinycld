import { describe, expect, it } from 'vitest'
import {
    compareVersions,
    detectDowngrade,
    type PackageVersionInfo,
} from '../../components/setup/use-package-versions'

function info(overrides: Partial<PackageVersionInfo>): PackageVersionInfo {
    return {
        slug: 'mail',
        source: 'npm',
        current: '1.2.0',
        latest: '1.3.0',
        available: ['1.3.0', '1.2.0', '1.1.0', '1.0.0'], // newest-first
        hasUpdate: true,
        ...overrides,
    }
}

describe('compareVersions', () => {
    it('compares numeric semver, tolerating a leading v and pre-release tags', () => {
        expect(compareVersions('1.2.0', '1.0.0')).toBeGreaterThan(0)
        expect(compareVersions('1.0.0', '1.2.0')).toBeLessThan(0)
        expect(compareVersions('1.0.0', '1.0.0')).toBe(0)
        expect(compareVersions('1.0', '1.0.0')).toBe(0)
        expect(compareVersions('v2.0.0', '1.9.9')).toBeGreaterThan(0)
        expect(compareVersions('1.2.3-beta.1', '1.2.3')).toBe(0) // pre-release ignored
    })
    it('returns null for unparseable input', () => {
        expect(compareVersions('garbage', '1.0.0')).toBeNull()
        expect(compareVersions('1.0.0', '')).toBeNull()
    })
})

describe('detectDowngrade', () => {
    it('uses the published available order when both versions are present', () => {
        const i = info({ current: '1.2.0' })
        expect(detectDowngrade(i, '1.0.0')).toBe(true) // older
        expect(detectDowngrade(i, '1.3.0')).toBe(false) // newer
    })

    it('falls back to a semver compare when a version is absent from available', () => {
        // current was yanked → not in available; target 1.0.0 < 1.2.0 is a downgrade.
        const i = info({ current: '1.2.0', available: ['1.3.0', '1.0.0'] })
        expect(detectDowngrade(i, '1.0.0')).toBe(true)
        expect(detectDowngrade(i, '1.3.0')).toBe(false)
    })

    it('treats an undeterminable direction as a downgrade (requires confirmation)', () => {
        // Neither current nor target parses and neither is indexable → must NOT be
        // silently treated as a safe upgrade. This is the H1/M3 safety default.
        const i = info({ current: 'weird-build', available: [] })
        expect(detectDowngrade(i, 'also-weird')).toBe(true)
    })
})
