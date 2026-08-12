import { describe, expect, it } from 'vitest'
import { buildConfigSource, buildSeedsSource, type ConfigPkg } from '../gen-config'

const contacts: ConfigPkg = {
    packageName: '@tinycld/contacts',
    slug: 'contacts',
    schemaType: 'ContactsSchema',
    hasRegister: true,
    hasSidebar: true,
    hasProvider: false,
    hasSeed: true,
    settings: [],
    systemSettings: [],
    slots: [],
    sidebarContributions: [],
    eventSources: [],
    eventSourceHost: false,
    manifest: { name: 'Contacts', slug: 'contacts', version: '0.1.0', description: 'd' },
}
const takeout: ConfigPkg = {
    packageName: '@tinycld/google-takeout-import',
    slug: 'google-takeout-import',
    schemaType: '',
    hasRegister: false,
    hasSidebar: false,
    hasProvider: false,
    hasSeed: false,
    settings: [{ slug: 'google-takeout', label: 'Import', component: 'settings/takeout' }],
    systemSettings: [],
    slots: [],
    sidebarContributions: [],
    eventSources: [],
    eventSourceHost: false,
    manifest: {
        name: 'Takeout',
        slug: 'google-takeout-import',
        version: '0.1.0',
        description: 'd',
    },
}

describe('buildConfigSource', () => {
    it('emits a definePackageEntry array with imports and a MergedPackageSchema', () => {
        const src = buildConfigSource([contacts])
        expect(src).toContain(
            "import { definePackageEntry } from '@tinycld/core/lib/packages/config-types'"
        )
        expect(src).toContain(
            "import { registerCollections as contactsRegister } from '@tinycld/contacts/collections'"
        )
        expect(src).toContain("import type { ContactsSchema } from '@tinycld/contacts/types'")
        expect(src).toContain('definePackageEntry<ContactsSchema>()({')
        expect(src).toContain('registerCollections: contactsRegister,')
        expect(src).toContain("sidebar: lazy(() => import('@tinycld/contacts/sidebar')),")
        expect(src).toContain('export type MergedPackageSchema = ContactsSchema')
    })

    it('handles settings-only packages (no register) with Record<string, never>', () => {
        const src = buildConfigSource([takeout])
        expect(src).toContain('definePackageEntry<Record<string, never>>()({')
        expect(src).toContain(
            "Component: lazy(() => import('@tinycld/google-takeout-import/settings/takeout'))"
        )
        // No schema in the merge → MergedPackageSchema falls back to Record<string, never>
        expect(src).toContain('export type MergedPackageSchema = Record<string, never>')
    })

    it('emits systemSettings as a lazy-loaded array (system-scoped panels)', () => {
        const withSystem: ConfigPkg = {
            ...takeout,
            systemSettings: [
                { slug: 'provider', label: 'Mail Provider', component: 'system-settings/provider' },
            ],
        }
        const src = buildConfigSource([withSystem])
        expect(src).toContain('systemSettings: [')
        expect(src).toContain('label: "Mail Provider"')
        expect(src).toContain(
            "lazy(() => import('@tinycld/google-takeout-import/system-settings/provider'))"
        )
    })

    it('camelCases slugs for identifiers', () => {
        const src = buildConfigSource([
            {
                ...contacts,
                slug: 'google-takeout-import',
                schemaType: 'GtiSchema',
                packageName: '@tinycld/google-takeout-import',
            },
        ])
        expect(src).toContain('as googleTakeoutImportRegister')
    })

    it('emits a lazy provider entry for hasProvider packages', () => {
        // Providers are emitted lazily (matching sidebar/settings) so the
        // generated config doesn't pull every package's provider module —
        // and its transitive screen tree — into pocketbase.ts's eager import
        // graph, which used to form a require cycle.
        const withProvider: ConfigPkg = {
            packageName: '@tinycld/drive',
            slug: 'drive',
            schemaType: 'DriveSchema',
            hasRegister: true,
            hasSidebar: false,
            hasProvider: true,
            hasSeed: false,
            settings: [],
            systemSettings: [],
            slots: [],
            sidebarContributions: [],
            eventSources: [],
            eventSourceHost: false,
            manifest: { name: 'Drive', slug: 'drive', version: '0.1.0', description: 'd' },
        }
        const src = buildConfigSource([withProvider])
        expect(src).toContain("import { lazy } from 'react'")
        expect(src).not.toContain("from '@tinycld/drive/provider'")
        expect(src).toContain("provider: lazy(() => import('@tinycld/drive/provider')),")
    })

    it('joins multiple package schemas into the MergedPackageSchema intersection', () => {
        const mail: ConfigPkg = {
            packageName: '@tinycld/mail',
            slug: 'mail',
            schemaType: 'MailSchema',
            hasRegister: true,
            hasSidebar: false,
            hasProvider: false,
            hasSeed: false,
            settings: [],
            systemSettings: [],
            slots: [],
            sidebarContributions: [],
            eventSources: [],
            eventSourceHost: false,
            manifest: { name: 'Mail', slug: 'mail', version: '0.1.0', description: 'd' },
        }
        const src = buildConfigSource([contacts, mail])
        expect(src).toContain('export type MergedPackageSchema = ContactsSchema & MailSchema')
    })

    it('throws when hasRegister is true but schemaType is empty', () => {
        const bad: ConfigPkg = { ...contacts, schemaType: '' }
        expect(() => buildConfigSource([bad])).toThrow(/schemaType is empty/)
    })

    it('rejects a packageName that would break out of the generated import string', () => {
        const bad: ConfigPkg = { ...contacts, packageName: "@tinycld/x'; evil()//" }
        expect(() => buildConfigSource([bad])).toThrow(/unsafe value/)
    })

    it('rejects a settings component subpath containing a quote', () => {
        const bad: ConfigPkg = {
            ...takeout,
            settings: [{ slug: 's', label: 'S', component: "settings/x')//" }],
        }
        expect(() => buildConfigSource([bad])).toThrow(/unsafe value/)
    })

    it('rejects a schemaType containing a backslash', () => {
        const bad: ConfigPkg = { ...contacts, schemaType: 'Contacts\\Schema' }
        expect(() => buildConfigSource([bad])).toThrow(/unsafe value/)
    })

    it('emits sidebarContributions as a lazy-loaded array with order', () => {
        const slotsPkg: ConfigPkg = {
            ...contacts,
            packageName: '@tinycld/calendar-slots',
            slug: 'calendar-slots',
            schemaType: '',
            hasRegister: false,
            hasSidebar: false,
            hasSeed: false,
            sidebarContributions: [
                {
                    target: 'calendar',
                    slot: 'sidebar.after-calendars',
                    component: 'sidebar-contributions/booking-pages',
                    order: 0,
                },
            ],
            manifest: {
                name: 'Calendar Slots',
                slug: 'calendar-slots',
                version: '0.1.0',
                description: 'd',
            },
        }
        const src = buildConfigSource([slotsPkg])
        expect(src).toContain("import { lazy } from 'react'")
        expect(src).toContain('sidebarContributions: [')
        expect(src).toContain('target: "calendar"')
        expect(src).toContain('slot: "sidebar.after-calendars"')
        expect(src).toContain('order: 0')
        expect(src).toContain(
            "Component: lazy(() => import('@tinycld/calendar-slots/sidebar-contributions/booking-pages'))"
        )
    })

    it('emits eventSources with a bare load thunk, never lazy()', () => {
        const cardsPkg: ConfigPkg = {
            ...contacts,
            packageName: '@tinycld/cards',
            slug: 'cards',
            schemaType: '',
            hasRegister: false,
            hasSidebar: false,
            hasSeed: false,
            eventSources: [
                {
                    target: 'calendar',
                    id: 'cards-due',
                    label: 'Card due dates',
                    module: 'calendar-source',
                    color: 'graphite',
                    order: 0,
                },
            ],
            manifest: { name: 'Cards', slug: 'cards', version: '0.1.0', description: 'd' },
        }
        const src = buildConfigSource([cardsPkg])
        expect(src).toContain('eventSources: [')
        expect(src).toContain(
            'target: "calendar", id: "cards-due", label: "Card due dates", color: "graphite", order: 0'
        )
        // A bare thunk: the module exports a hook, so React.lazy cannot wrap it.
        expect(src).toContain("load: () => import('@tinycld/cards/calendar-source')")
        expect(src).not.toContain("lazy(() => import('@tinycld/cards/calendar-source'))")
        // An event source alone must not pull in the react lazy import.
        expect(src).not.toContain("import { lazy } from 'react'")
    })

    it('rejects an eventSource module subpath containing a quote', () => {
        const bad: ConfigPkg = {
            ...takeout,
            eventSources: [
                {
                    target: 'calendar',
                    id: 'x',
                    label: 'X',
                    module: "calendar-source')//",
                    order: 0,
                },
            ],
        }
        expect(() => buildConfigSource([bad])).toThrow(/unsafe value/)
    })

    it('imports and attaches automation definitions when declared', () => {
        const withAutomation: ConfigPkg = { ...contacts, automation: 'automation' }
        const src = buildConfigSource([withAutomation])
        expect(src).toContain("import contactsAutomation from '@tinycld/contacts/automation'")
        expect(src).toContain('automation: contactsAutomation,')
    })

    it('omits automation entirely when not declared', () => {
        const src = buildConfigSource([contacts])
        expect(src).not.toContain('automation')
    })
})

describe('buildSeedsSource', () => {
    it('emits only packages with seeds, carrying dependencies', () => {
        const src = buildSeedsSource([contacts, takeout])
        expect(src).toContain("import contactsSeed from '@tinycld/contacts/seed'")
        expect(src).not.toContain('takeout')
        expect(src).toContain(
            '{ manifest: { slug: "contacts", dependencies: [] }, seed: contactsSeed },'
        )
    })
})
