import { describe, expect, it, vi } from 'vitest'
import {
    manifestToConfigPkg,
    schemaTypeName,
    validateSidebarContributions,
} from '../describe-packages'

describe('schemaTypeName', () => {
    it('PascalCases the slug + Schema', () => {
        expect(schemaTypeName('contacts')).toBe('ContactsSchema')
        expect(schemaTypeName('google-takeout-import')).toBe('GoogleTakeoutImportSchema')
    })
})

describe('manifestToConfigPkg', () => {
    it('derives flags from manifest presence', () => {
        const cp = manifestToConfigPkg('@tinycld/contacts', {
            name: 'Contacts',
            slug: 'contacts',
            version: '0.1.0',
            description: 'd',
            collections: { register: 'collections', types: 'types' },
            sidebar: { component: 'sidebar' },
            seed: { script: 'seed' },
            routes: { directory: 'screens' },
        })
        expect(cp.hasRegister).toBe(true)
        expect(cp.schemaType).toBe('ContactsSchema')
        expect(cp.hasSidebar).toBe(true)
        expect(cp.hasProvider).toBe(false)
        expect(cp.hasSeed).toBe(true)
        expect(cp.settings).toEqual([])
    })

    it('settings-only package has no register and empty schemaType', () => {
        const cp = manifestToConfigPkg('@tinycld/google-takeout-import', {
            name: 'T',
            slug: 'google-takeout-import',
            version: '0.1.0',
            description: 'd',
            settings: [{ slug: 'g', component: 'settings/takeout', label: 'Import' }],
        })
        expect(cp.hasRegister).toBe(false)
        expect(cp.schemaType).toBe('')
        expect(cp.settings).toEqual([{ slug: 'g', component: 'settings/takeout', label: 'Import' }])
        expect(cp.slots).toEqual([])
        expect(cp.sidebarContributions).toEqual([])
    })

    it('passes through slots and sidebarContributions, defaulting order to 0', () => {
        const cp = manifestToConfigPkg('@tinycld/calendar-slots', {
            name: 'Calendar Slots',
            slug: 'calendar-slots',
            version: '0.1.0',
            description: 'd',
            sidebarContributions: [
                {
                    target: 'calendar',
                    slot: 'sidebar.after-calendars',
                    component: 'sidebar-contributions/booking-pages',
                },
            ],
        })
        expect(cp.sidebarContributions).toEqual([
            {
                target: 'calendar',
                slot: 'sidebar.after-calendars',
                component: 'sidebar-contributions/booking-pages',
                order: 0,
            },
        ])
    })

    it('rejects duplicate slot names in manifest.slots', () => {
        expect(() =>
            manifestToConfigPkg('@tinycld/calendar', {
                name: 'Calendar',
                slug: 'calendar',
                version: '0.1.0',
                description: 'd',
                slots: ['sidebar.after-calendars', 'sidebar.after-calendars'],
            })
        ).toThrow(/duplicate slot name 'sidebar\.after-calendars'/)
    })
})

describe('validateSidebarContributions', () => {
    const calendarHost = manifestToConfigPkg('@tinycld/calendar', {
        name: 'Calendar',
        slug: 'calendar',
        version: '0.1.0',
        description: 'd',
        slots: ['sidebar.after-calendars'],
    })

    const validContributor = manifestToConfigPkg('@tinycld/calendar-slots', {
        name: 'Calendar Slots',
        slug: 'calendar-slots',
        version: '0.1.0',
        description: 'd',
        sidebarContributions: [
            {
                target: 'calendar',
                slot: 'sidebar.after-calendars',
                component: 'sidebar-contributions/booking-pages',
            },
        ],
    })

    it('accepts contributions targeting declared slots', () => {
        expect(() => validateSidebarContributions([calendarHost, validContributor])).not.toThrow()
    })

    it('rejects contributions targeting an unknown slot on a present host', () => {
        const badContributor = manifestToConfigPkg('@tinycld/calendar-slots', {
            name: 'Calendar Slots',
            slug: 'calendar-slots',
            version: '0.1.0',
            description: 'd',
            sidebarContributions: [
                {
                    target: 'calendar',
                    slot: 'sidebar.tpyo',
                    component: 'sidebar-contributions/booking-pages',
                },
            ],
        })
        expect(() => validateSidebarContributions([calendarHost, badContributor])).toThrow(
            /unknown slot 'calendar:sidebar\.tpyo'/
        )
    })

    it('tolerates contributions targeting an absent host (partial checkout)', () => {
        const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
        try {
            expect(() => validateSidebarContributions([validContributor])).not.toThrow()
            expect(warn).toHaveBeenCalledWith(
                expect.stringMatching(/not installed in this workspace/)
            )
        } finally {
            warn.mockRestore()
        }
    })
})
