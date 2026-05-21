import { describe, expect, it } from 'vitest'
import { deriveProviders, deriveSettings, deriveSidebars } from '../derive-components'

const A = () => null
const P = () => null

describe('derive-components', () => {
    it('maps slug -> sidebar (null when absent)', () => {
        const s = deriveSidebars([
            { manifest: { slug: 'contacts' }, sidebar: A },
            { manifest: { slug: 'calc' } },
        ] as never)
        expect(s.contacts).toBe(A)
        expect(s.calc).toBeNull()
    })

    it('maps slug -> provider (null when absent)', () => {
        const p = deriveProviders([
            { manifest: { slug: 'calc' }, provider: P },
            { manifest: { slug: 'contacts' } },
        ] as never)
        expect(p.calc).toBe(P)
        expect(p.contacts).toBeNull()
    })

    it('groups settings panels by package, skipping packages with none', () => {
        const g = deriveSettings([
            {
                manifest: { name: 'Mail', slug: 'mail' },
                settings: [{ slug: 'provider', label: 'Provider', Component: A }],
            },
            { manifest: { name: 'Calc', slug: 'calc' } },
        ] as never)
        expect(g).toHaveLength(1)
        expect(g[0].pkgSlug).toBe('mail')
        expect(g[0].packageName).toBe('Mail')
        expect(g[0].panels[0].slug).toBe('provider')
    })
})
