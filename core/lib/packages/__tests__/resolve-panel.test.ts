import { describe, expect, it } from 'vitest'
import { resolvePanel } from '../resolve-panel'

const A = () => null
const B = () => null

const orgGroups = [
    {
        pkgSlug: 'mail',
        packageName: 'Mail',
        panels: [
            { slug: 'provider', label: 'Domains', Component: A },
            { slug: 'mailboxes', label: 'Mailboxes', Component: A },
        ],
    },
]

const systemGroups = [
    {
        pkgSlug: 'mail',
        packageName: 'Mail',
        panels: [{ slug: 'provider', label: 'Provider', Component: B }],
    },
]

describe('resolvePanel', () => {
    it('finds the panel a package contributes', () => {
        const match = resolvePanel(orgGroups, 'mail', 'mailboxes')
        expect(match?.panel.label).toBe('Mailboxes')
        expect(match?.group.packageName).toBe('Mail')
    })

    it('returns null for an unknown package', () => {
        expect(resolvePanel(orgGroups, 'calendar', 'provider')).toBeNull()
    })

    it('returns null for a slug the package does not contribute', () => {
        expect(resolvePanel(orgGroups, 'mail', 'nope')).toBeNull()
    })

    it('returns null when either segment is missing', () => {
        expect(resolvePanel(orgGroups, 'mail', undefined)).toBeNull()
        expect(resolvePanel(orgGroups, undefined, 'provider')).toBeNull()
        expect(resolvePanel(orgGroups, undefined, undefined)).toBeNull()
    })

    // Why the two settings route trees stay separate. Mail declares the slug
    // `provider` in BOTH its org-scoped `settings` and its deployment-wide
    // `systemSettings`. The same (pkgSlug, panelSlug) pair must therefore
    // resolve to a DIFFERENT panel depending on which registry is searched —
    // merging the registries would strand whichever panel lost the lookup.
    it('resolves the same slug pair to a different panel per registry', () => {
        const org = resolvePanel(orgGroups, 'mail', 'provider')
        const system = resolvePanel(systemGroups, 'mail', 'provider')

        expect(org?.panel.label).toBe('Domains')
        expect(system?.panel.label).toBe('Provider')
        expect(org?.panel.Component).not.toBe(system?.panel.Component)
    })
})
