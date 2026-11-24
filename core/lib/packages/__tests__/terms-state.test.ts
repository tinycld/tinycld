import { describe, expect, it } from 'vitest'
import {
    blockedSlugsOf,
    hasUndecidedTerms,
    openEntriesOf,
    pendingTermsOf,
    termsStateOf,
} from '../terms-state'

const row = (slug: string, version: string, accepted: string) => ({
    slug,
    terms_version: version,
    terms_accepted_version: accepted,
})

describe('termsStateOf', () => {
    it('reads a package with no terms as none', () => {
        expect(termsStateOf(row('acme', '', ''))).toBe('none')
        expect(termsStateOf(undefined)).toBe('none')
    })
    it('reads an accepted current version as accepted', () => {
        expect(termsStateOf(row('acme', '2026-11-01', '2026-11-01'))).toBe('accepted')
    })
    it('reads a new terms version as pending again', () => {
        expect(termsStateOf(row('acme', '2026-12-01', '2026-11-01'))).toBe('pending')
        expect(termsStateOf(row('acme', '2026-11-01', ''))).toBe('pending')
    })
    it('ignores an accepted version left behind by a package that dropped its terms', () => {
        expect(termsStateOf(row('acme', '', '2026-11-01'))).toBe('none')
    })
})

describe('blockedSlugsOf', () => {
    const rows = [row('acme', 'v2', 'v1'), row('gizmos', 'v1', 'v1')]
    it('closes every package that declares terms until the registry has loaded', () => {
        expect([...blockedSlugsOf(['acme', 'gizmos'], [], false)]).toEqual(['acme', 'gizmos'])
    })
    it('closes only pending packages once the registry has loaded', () => {
        expect([...blockedSlugsOf(['acme', 'gizmos'], rows, true)]).toEqual(['acme'])
    })
    it('closes nothing when no package declares terms', () => {
        expect(blockedSlugsOf([], [], false).size).toBe(0)
        expect(blockedSlugsOf([], [], true).size).toBe(0)
    })
    it('opens a declared package that has no registry row', () => {
        expect(blockedSlugsOf(['zoo'], [], true).size).toBe(0)
    })
})

describe('pendingTermsOf', () => {
    const names = (slug: string) => ({ acme: 'Acme', gizmos: 'Gizmos' })[slug] ?? slug
    it('lists pending packages and marks a decline of the current version', () => {
        const rows = [row('acme', 'v2', 'v1'), row('gizmos', 'v1', ''), row('zoo', 'v1', 'v1')]
        const declined = [
            { pkg: 'gizmos', terms_version: 'v1' },
            // A decline of an older version does not count for the new one.
            { pkg: 'acme', terms_version: 'v1' },
        ]
        expect(pendingTermsOf(rows, declined, names)).toEqual([
            { slug: 'acme', name: 'Acme', version: 'v2', isDeclined: false },
            { slug: 'gizmos', name: 'Gizmos', version: 'v1', isDeclined: true },
        ])
    })
    it('asks the owner only while a pending package has no decision', () => {
        const declined = [{ slug: 'a', name: 'A', version: 'v1', isDeclined: true }]
        expect(hasUndecidedTerms(declined)).toBe(false)
        expect(
            hasUndecidedTerms([
                ...declined,
                { slug: 'b', name: 'B', version: 'v1', isDeclined: false },
            ])
        ).toBe(true)
        expect(hasUndecidedTerms([])).toBe(false)
    })
})

describe('openEntriesOf', () => {
    const groups = [{ pkgSlug: 'acme' }, { pkgSlug: 'gizmos' }]
    it('drops the entries of closed packages', () => {
        expect(openEntriesOf(groups, g => g.pkgSlug, new Set(['acme']))).toEqual([
            { pkgSlug: 'gizmos' },
        ])
    })
    it('returns the same list when nothing is closed', () => {
        expect(openEntriesOf(groups, g => g.pkgSlug, new Set())).toBe(groups)
    })
})
