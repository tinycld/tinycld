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
    it('recognizes a single-tenant server build id', () => {
        expect(classifyBundleId('build-1718200000000-ios')).toBe('server')
        expect(classifyBundleId('build-1718200000000-android')).toBe('server')
    })
    it('recognizes a multi-org content-addressed build id', () => {
        // The multi-org builder mints recipe-<hash12>-<platform>, so two orgs
        // with the same package set advertise the SAME bundle. Without this the
        // hosted OTA harness classifies every real id as 'unknown' and fails at
        // precheck.
        expect(classifyBundleId('recipe-ab12cd34ef56-ios')).toBe('server')
        expect(classifyBundleId('recipe-ab12cd34ef56-android')).toBe('server')
    })
    it('returns unknown for anything else', () => {
        expect(classifyBundleId('')).toBe('unknown')
        expect(classifyBundleId('garbage')).toBe('unknown')
        // Recipe-shaped but not a 12-char hex digest — must not be trusted as a
        // server id (the server-side path validator is equally strict).
        expect(classifyBundleId('recipe-zzzzzzzzzzzz-ios')).toBe('unknown')
        expect(classifyBundleId('recipe-abc-ios')).toBe('unknown')
    })
})
