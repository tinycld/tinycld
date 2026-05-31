import { describe, expect, it } from 'vitest'
import {
    deriveProviders,
    deriveSettings,
    deriveSidebarContributions,
    deriveSidebars,
} from '../derive-components'

const A = () => null
const P = () => null
const C1 = () => null
const C2 = () => null
const C3 = () => null

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

    describe('deriveSidebarContributions', () => {
        it('returns an empty registry for an empty config', () => {
            expect(deriveSidebarContributions([] as never)).toEqual({})
        })

        it('groups by target slug and slot name', () => {
            const r = deriveSidebarContributions([
                {
                    manifest: { slug: 'calendar-slots' },
                    sidebarContributions: [
                        {
                            target: 'calendar',
                            slot: 'sidebar.after-calendars',
                            order: 0,
                            Component: C1,
                        },
                    ],
                },
                {
                    manifest: { slug: 'drive-notes' },
                    sidebarContributions: [
                        {
                            target: 'drive',
                            slot: 'sidebar.after-tree',
                            order: 0,
                            Component: C2,
                        },
                    ],
                },
            ] as never)
            expect(r.calendar['sidebar.after-calendars']).toHaveLength(1)
            expect(r.calendar['sidebar.after-calendars'][0].Component).toBe(C1)
            expect(r.calendar['sidebar.after-calendars'][0].contributorSlug).toBe('calendar-slots')
            expect(r.drive['sidebar.after-tree'][0].Component).toBe(C2)
        })

        it('orders by `order` ascending, then by contributor slug as tiebreaker', () => {
            const r = deriveSidebarContributions([
                {
                    manifest: { slug: 'zeta' },
                    sidebarContributions: [
                        { target: 'calendar', slot: 'x', order: 0, Component: C3 },
                    ],
                },
                {
                    manifest: { slug: 'alpha' },
                    sidebarContributions: [
                        { target: 'calendar', slot: 'x', order: 0, Component: C1 },
                    ],
                },
                {
                    manifest: { slug: 'beta' },
                    sidebarContributions: [
                        { target: 'calendar', slot: 'x', order: -10, Component: C2 },
                    ],
                },
            ] as never)
            // -10 (beta) sorts first; then 0 ties broken alpha < zeta.
            expect(r.calendar.x.map(e => e.contributorSlug)).toEqual(['beta', 'alpha', 'zeta'])
        })

        it('skips entries without sidebarContributions', () => {
            const r = deriveSidebarContributions([
                { manifest: { slug: 'calc' } },
                {
                    manifest: { slug: 'calendar-slots' },
                    sidebarContributions: [
                        {
                            target: 'calendar',
                            slot: 'sidebar.after-calendars',
                            order: 0,
                            Component: C1,
                        },
                    ],
                },
            ] as never)
            expect(Object.keys(r)).toEqual(['calendar'])
        })
    })
})
