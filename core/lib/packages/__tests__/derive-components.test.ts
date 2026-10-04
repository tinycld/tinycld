import { CORE_SLOT_TARGET } from '@tinycld/core/lib/setup/core-slots'
import { describe, expect, it } from 'vitest'
import {
    deriveAccountSettings,
    deriveProviders,
    deriveSettings,
    deriveSidebarContributions,
    deriveSidebars,
    deriveSystemSettings,
} from '../derive-components'

const A = () => null
const P = () => null
const C1 = () => null
const C2 = () => null
const C3 = () => null

describe('derive-components', () => {
    it('maps slug -> sidebar (null when absent)', () => {
        const s = deriveSidebars([
            { manifest: { slug: 'doodads' }, sidebar: A },
            { manifest: { slug: 'trinkets' } },
        ] as never)
        expect(s.doodads).toBe(A)
        expect(s.trinkets).toBeNull()
    })

    it('maps slug -> provider (null when absent)', () => {
        const p = deriveProviders([
            { manifest: { slug: 'trinkets' }, provider: P },
            { manifest: { slug: 'doodads' } },
        ] as never)
        expect(p.trinkets).toBe(P)
        expect(p.doodads).toBeNull()
    })

    it('groups settings panels by package, skipping packages with none', () => {
        const g = deriveSettings([
            {
                manifest: { name: 'Gizmos', slug: 'gizmos' },
                settings: [{ slug: 'provider', label: 'Provider', Component: A }],
            },
            { manifest: { name: 'Trinkets', slug: 'trinkets' } },
        ] as never)
        expect(g).toHaveLength(1)
        expect(g[0].pkgSlug).toBe('gizmos')
        expect(g[0].packageName).toBe('Gizmos')
        expect(g[0].panels[0].slug).toBe('provider')
    })

    it('groups account panels by package, ignoring org-scoped settings', () => {
        const g = deriveAccountSettings([
            {
                manifest: { name: 'Gizmos', slug: 'gizmos', nav: { icon: 'mail' } },
                accountSettings: [{ slug: 'import', label: 'Import', Component: A }],
            },
            {
                manifest: { name: 'Cogs', slug: 'cogs' },
                settings: [{ slug: 'x', label: 'X', Component: P }],
            },
        ] as never)
        expect(g).toHaveLength(1)
        expect(g[0].pkgSlug).toBe('gizmos')
        expect(g[0].icon).toBe('mail')
        expect(g[0].panels[0].Component).toBe(A)
    })

    it('groups system-settings panels by package, skipping packages with none', () => {
        const g = deriveSystemSettings([
            {
                manifest: { name: 'Gizmos', slug: 'gizmos', nav: { icon: 'mail' } },
                systemSettings: [{ slug: 'provider', label: 'Gizmos Provider', Component: A }],
            },
            { manifest: { name: 'Trinkets', slug: 'trinkets' } },
            // settings (org-scoped) present but no systemSettings → still skipped
            {
                manifest: { name: 'Cogs', slug: 'cogs' },
                settings: [{ slug: 'x', label: 'X', Component: P }],
            },
        ] as never)
        expect(g).toHaveLength(1)
        expect(g[0].pkgSlug).toBe('gizmos')
        expect(g[0].packageName).toBe('Gizmos')
        expect(g[0].icon).toBe('mail')
        expect(g[0].panels[0].slug).toBe('provider')
        expect(g[0].panels[0].Component).toBe(A)
    })

    describe('deriveSidebarContributions', () => {
        it('returns an empty registry for an empty config', () => {
            expect(deriveSidebarContributions([] as never)).toEqual({})
        })

        it('groups by target slug and slot name', () => {
            const r = deriveSidebarContributions([
                {
                    manifest: { slug: 'sprockets-slots' },
                    sidebarContributions: [
                        {
                            target: 'sprockets',
                            slot: 'sidebar.after-calendars',
                            order: 0,
                            Component: C1,
                        },
                    ],
                },
                {
                    manifest: { slug: 'cogs-notes' },
                    sidebarContributions: [
                        {
                            target: 'cogs',
                            slot: 'sidebar.after-tree',
                            order: 0,
                            Component: C2,
                        },
                    ],
                },
            ] as never)
            expect(r.sprockets['sidebar.after-calendars']).toHaveLength(1)
            expect(r.sprockets['sidebar.after-calendars'][0].Component).toBe(C1)
            expect(r.sprockets['sidebar.after-calendars'][0].contributorSlug).toBe(
                'sprockets-slots'
            )
            expect(r.cogs['sidebar.after-tree'][0].Component).toBe(C2)
        })

        it('carries a contribution label through', () => {
            const r = deriveSidebarContributions([
                {
                    manifest: { slug: 'sprockets-slots' },
                    sidebarContributions: [
                        {
                            target: 'sprockets',
                            slot: 'setup-options',
                            order: 0,
                            label: 'Built-in',
                            Component: C1,
                        },
                    ],
                },
            ] as never)
            expect(r.sprockets['setup-options'][0].label).toBe('Built-in')
        })

        it('orders by `order` ascending, then by contributor slug as tiebreaker', () => {
            const r = deriveSidebarContributions([
                {
                    manifest: { slug: 'zeta' },
                    sidebarContributions: [
                        { target: 'sprockets', slot: 'x', order: 0, Component: C3 },
                    ],
                },
                {
                    manifest: { slug: 'alpha' },
                    sidebarContributions: [
                        { target: 'sprockets', slot: 'x', order: 0, Component: C1 },
                    ],
                },
                {
                    manifest: { slug: 'beta' },
                    sidebarContributions: [
                        { target: 'sprockets', slot: 'x', order: -10, Component: C2 },
                    ],
                },
            ] as never)
            // -10 (beta) sorts first; then 0 ties broken alpha < zeta.
            expect(r.sprockets.x.map(e => e.contributorSlug)).toEqual(['beta', 'alpha', 'zeta'])
        })

        it('skips entries without sidebarContributions', () => {
            const r = deriveSidebarContributions([
                { manifest: { slug: 'trinkets' } },
                {
                    manifest: { slug: 'sprockets-slots' },
                    sidebarContributions: [
                        {
                            target: 'sprockets',
                            slot: 'sidebar.after-calendars',
                            order: 0,
                            Component: C1,
                        },
                    ],
                },
            ] as never)
            expect(Object.keys(r)).toEqual(['sprockets'])
        })

        it('files contributions that target core under the core slot target', () => {
            const r = deriveSidebarContributions([
                {
                    manifest: { slug: 'seats' },
                    sidebarContributions: [
                        {
                            target: CORE_SLOT_TARGET,
                            slot: 'setup-team',
                            order: 0,
                            Component: C1,
                        },
                    ],
                },
            ] as never)
            expect(r[CORE_SLOT_TARGET]['setup-team'][0].Component).toBe(C1)
        })
    })
})
