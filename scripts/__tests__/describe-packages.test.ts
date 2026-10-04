import { describe, expect, it, vi } from 'vitest'
import type { PortsFeature } from '../describe-packages'
import {
    manifestToConfigPkg,
    schemaTypeName,
    validateEventSources,
    validateNavShortcuts,
    validatePorts,
    validateSidebarContributions,
} from '../describe-packages'

describe('schemaTypeName', () => {
    it('PascalCases the slug + Schema', () => {
        expect(schemaTypeName('doodads')).toBe('DoodadsSchema')
        expect(schemaTypeName('gizmo-import')).toBe('GizmoImportSchema')
    })
})

describe('manifestToConfigPkg', () => {
    it('derives flags from manifest presence', () => {
        const cp = manifestToConfigPkg('@tinycld/doodads', {
            name: 'Doodads',
            slug: 'doodads',
            version: '0.1.0',
            description: 'd',
            collections: { register: 'collections', types: 'types' },
            sidebar: { component: 'sidebar' },
            seed: { script: 'seed' },
            routes: { directory: 'screens' },
        })
        expect(cp.hasRegister).toBe(true)
        expect(cp.schemaType).toBe('DoodadsSchema')
        expect(cp.hasSidebar).toBe(true)
        expect(cp.hasProvider).toBe(false)
        expect(cp.hasSeed).toBe(true)
        expect(cp.settings).toEqual([])
    })

    it('settings-only package has no register and empty schemaType', () => {
        const cp = manifestToConfigPkg('@tinycld/gizmo-import', {
            name: 'T',
            slug: 'gizmo-import',
            version: '0.1.0',
            description: 'd',
            settings: [{ slug: 'g', component: 'settings/takeout', label: 'Import' }],
            systemSettings: [
                { slug: 'mail', component: 'system-settings/provider', label: 'Mail Provider' },
            ],
        })
        expect(cp.hasRegister).toBe(false)
        expect(cp.schemaType).toBe('')
        expect(cp.settings).toEqual([{ slug: 'g', component: 'settings/takeout', label: 'Import' }])
        expect(cp.systemSettings).toEqual([
            { slug: 'mail', component: 'system-settings/provider', label: 'Mail Provider' },
        ])
        expect(cp.slots).toEqual([])
        expect(cp.sidebarContributions).toEqual([])
    })

    it('defaults systemSettings to [] when the manifest omits it', () => {
        const cp = manifestToConfigPkg('@tinycld/doodads', {
            name: 'Doodads',
            slug: 'doodads',
            version: '0.1.0',
            description: 'd',
        })
        expect(cp.systemSettings).toEqual([])
    })

    it('passes through slots and sidebarContributions, defaulting order to 0', () => {
        const cp = manifestToConfigPkg('@tinycld/sprockets-slots', {
            name: 'Sprockets Slots',
            slug: 'sprockets-slots',
            version: '0.1.0',
            description: 'd',
            sidebarContributions: [
                {
                    target: 'sprockets',
                    slot: 'sidebar.after-sprockets',
                    component: 'sidebar-contributions/booking-pages',
                },
            ],
        })
        expect(cp.sidebarContributions).toEqual([
            {
                target: 'sprockets',
                slot: 'sidebar.after-sprockets',
                component: 'sidebar-contributions/booking-pages',
                order: 0,
            },
        ])
    })

    it('rejects duplicate slot names in manifest.slots', () => {
        expect(() =>
            manifestToConfigPkg('@tinycld/sprockets', {
                name: 'Sprockets',
                slug: 'sprockets',
                version: '0.1.0',
                description: 'd',
                slots: ['sidebar.after-sprockets', 'sidebar.after-sprockets'],
            })
        ).toThrow(/duplicate slot name 'sidebar\.after-sprockets'/)
    })

    it('maps manifest.automation.definitions to ConfigPkg.automation', () => {
        const pkg = manifestToConfigPkg('@tinycld/doodads', {
            name: 'Doodads',
            slug: 'doodads',
            version: '0.1.0',
            description: 'd',
            automation: { definitions: 'automation' },
        })
        expect(pkg.automation).toBe('automation')
    })

    it('leaves ConfigPkg.automation undefined when the manifest has none', () => {
        const pkg = manifestToConfigPkg('@tinycld/doodads', {
            name: 'Doodads',
            slug: 'doodads',
            version: '0.1.0',
            description: 'd',
        })
        expect(pkg.automation).toBeUndefined()
    })
})

describe('validateSidebarContributions', () => {
    const sprocketsHost = manifestToConfigPkg('@tinycld/sprockets', {
        name: 'Sprockets',
        slug: 'sprockets',
        version: '0.1.0',
        description: 'd',
        slots: ['sidebar.after-sprockets'],
    })

    const validContributor = manifestToConfigPkg('@tinycld/sprockets-slots', {
        name: 'Sprockets Slots',
        slug: 'sprockets-slots',
        version: '0.1.0',
        description: 'd',
        sidebarContributions: [
            {
                target: 'sprockets',
                slot: 'sidebar.after-sprockets',
                component: 'sidebar-contributions/booking-pages',
            },
        ],
    })

    it('accepts contributions targeting declared slots', () => {
        expect(() => validateSidebarContributions([sprocketsHost, validContributor])).not.toThrow()
    })

    it('rejects contributions targeting an unknown slot on a present host', () => {
        const badContributor = manifestToConfigPkg('@tinycld/sprockets-slots', {
            name: 'Sprockets Slots',
            slug: 'sprockets-slots',
            version: '0.1.0',
            description: 'd',
            sidebarContributions: [
                {
                    target: 'sprockets',
                    slot: 'sidebar.tpyo',
                    component: 'sidebar-contributions/booking-pages',
                },
            ],
        })
        expect(() => validateSidebarContributions([sprocketsHost, badContributor])).toThrow(
            /unknown slot 'sprockets:sidebar\.tpyo'/
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

describe('validateEventSources', () => {
    const source = (overrides: Partial<{ target: string; id: string }> = {}) => ({
        target: 'sprockets',
        id: 'gadgets-due',
        label: 'Card due dates',
        module: 'sprockets-source',
        ...overrides,
    })

    const contributor = (slug: string, sources = [source()]) =>
        manifestToConfigPkg(`@tinycld/${slug}`, {
            name: slug,
            slug,
            version: '0.1.0',
            description: 'd',
            eventSources: sources,
        })

    const host = manifestToConfigPkg('@tinycld/sprockets', {
        name: 'Sprockets',
        slug: 'sprockets',
        version: '0.1.0',
        description: 'd',
        eventSourceHost: true,
    })

    it('maps eventSources onto ConfigPkg, defaulting order to 0', () => {
        const cp = contributor('gadgets')
        expect(cp.eventSources).toEqual([
            {
                target: 'sprockets',
                id: 'gadgets-due',
                label: 'Card due dates',
                module: 'sprockets-source',
                order: 0,
            },
        ])
        expect(cp.eventSourceHost).toBe(false)
        expect(host.eventSourceHost).toBe(true)
    })

    it('accepts a source targeting a present host', () => {
        expect(() => validateEventSources([host, contributor('gadgets')])).not.toThrow()
    })

    it('tolerates a source targeting an absent host (partial checkout)', () => {
        const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
        try {
            expect(() => validateEventSources([contributor('gadgets')])).not.toThrow()
            expect(warn).toHaveBeenCalledWith(
                expect.stringMatching(/not installed in this workspace/)
            )
        } finally {
            warn.mockRestore()
        }
    })

    it('rejects a source targeting a present package that is not a host', () => {
        const nonHost = manifestToConfigPkg('@tinycld/sprockets', {
            name: 'Sprockets',
            slug: 'sprockets',
            version: '0.1.0',
            description: 'd',
        })
        expect(() => validateEventSources([nonHost, contributor('gadgets')])).toThrow(
            /does not declare eventSourceHost/
        )
    })

    it('rejects an id outside [a-z0-9-]', () => {
        expect(() =>
            validateEventSources([host, contributor('gadgets', [source({ id: 'Cards:Due' })])])
        ).toThrow(/must match \[a-z0-9-\]\+/)
    })

    it('rejects a duplicate (target, id) across contributors', () => {
        expect(() =>
            validateEventSources([host, contributor('gadgets'), contributor('widgets')])
        ).toThrow(/declared by both 'gadgets' and 'widgets'/)
    })
})

describe('setupSteps', () => {
    const base = { name: 'Acme', slug: 'acme', version: '1.0.0', description: 'x' }

    it('carries setup steps with their order', () => {
        const pkg = manifestToConfigPkg('@acme/acme', {
            ...base,
            setupSteps: [{ id: 'plan', label: 'Plan', module: 'setup/plan', order: 'Zz' }],
        })
        expect(pkg.setupSteps).toEqual([
            { id: 'plan', label: 'Plan', module: 'setup/plan', order: 'Zz' },
        ])
    })

    it('rejects an invalid order key', () => {
        expect(() =>
            manifestToConfigPkg('@acme/acme', {
                ...base,
                setupSteps: [{ id: 'plan', label: 'Plan', module: 'setup/plan', order: '10' }],
            })
        ).toThrow(/order '10'/)
    })

    it('rejects a step id outside [a-z0-9-]', () => {
        expect(() =>
            manifestToConfigPkg('@acme/acme', {
                ...base,
                setupSteps: [{ id: 'Plan.x', label: 'Plan', module: 'setup/plan' }],
            })
        ).toThrow(/id 'Plan\.x'/)
    })
})

describe('core slot contributions', () => {
    const pkg = (slot: string) =>
        manifestToConfigPkg('@acme/acme', {
            name: 'Acme',
            slug: 'acme',
            version: '1.0.0',
            description: 'x',
            sidebarContributions: [{ target: 'core', slot, component: 'setup/seats' }],
        })

    it('accepts a slot core declares', () => {
        expect(() => validateSidebarContributions([pkg('setup-team')])).not.toThrow()
    })
    it('rejects an unknown core slot', () => {
        expect(() => validateSidebarContributions([pkg('nope')])).toThrow(
            /unknown slot 'core:nope'/
        )
    })
})

describe('validateNavShortcuts', () => {
    const withShortcut = (slug: string, shortcut?: string) =>
        manifestToConfigPkg(`@tinycld/${slug}`, {
            name: slug,
            slug,
            version: '0.1.0',
            description: 'd',
            nav: { label: slug, icon: 'box', order: 1, ...(shortcut ? { shortcut } : {}) },
        })

    it('accepts distinct letters', () => {
        expect(() =>
            validateNavShortcuts([withShortcut('gizmos', 'm'), withShortcut('gadgets', 'k')])
        ).not.toThrow()
    })

    it('rejects two packages claiming the same letter', () => {
        expect(() =>
            validateNavShortcuts([withShortcut('gadgets', 'k'), withShortcut('kanban', 'k')])
        ).toThrow(/'k' is claimed by both 'gadgets' and 'kanban'/)
    })

    it('ignores packages that declare no shortcut', () => {
        expect(() =>
            validateNavShortcuts([withShortcut('a'), withShortcut('b'), withShortcut('c', 'c')])
        ).not.toThrow()
    })
})

describe('validatePorts', () => {
    const feature = (slug: string, ports: PortsFeature['ports']): PortsFeature => ({
        slug,
        ports,
    })

    it('accepts a valid set', () => {
        expect(() =>
            validatePorts([
                feature('acme', [{ name: 'acme-sync', port: 1234 }]),
                feature('zeta', [{ name: 'zeta-sync', port: 5678 }]),
            ])
        ).not.toThrow()
    })

    it('tolerates packages that declare no ports', () => {
        expect(() => validatePorts([feature('acme', undefined)])).not.toThrow()
    })

    it('rejects two packages declaring the same port', () => {
        expect(() =>
            validatePorts([
                feature('acme', [{ name: 'acme-sync', port: 1234 }]),
                feature('zeta', [{ name: 'zeta-sync', port: 1234 }]),
            ])
        ).toThrow(/'acme' and 'zeta'/)
    })

    it('rejects two packages declaring the same name', () => {
        expect(() =>
            validatePorts([
                feature('acme', [{ name: 'shared-name', port: 1234 }]),
                feature('zeta', [{ name: 'shared-name', port: 5678 }]),
            ])
        ).toThrow(/'acme' and 'zeta'/)
    })

    it('rejects port 0', () => {
        expect(() => validatePorts([feature('acme', [{ name: 'acme-sync', port: 0 }])])).toThrow(
            /1-65535/
        )
    })

    it('rejects port 70000', () => {
        expect(() =>
            validatePorts([feature('acme', [{ name: 'acme-sync', port: 70000 }])])
        ).toThrow(/1-65535/)
    })

    it('rejects an uppercase name', () => {
        expect(() => validatePorts([feature('acme', [{ name: 'ACME-SYNC', port: 1234 }])])).toThrow(
            /\[a-z0-9-\]\+/
        )
    })

    it('rejects the reserved name http', () => {
        expect(() => validatePorts([feature('acme', [{ name: 'http', port: 1234 }])])).toThrow(
            /reserved/
        )
    })

    it('rejects the reserved name https', () => {
        expect(() => validatePorts([feature('acme', [{ name: 'https', port: 1234 }])])).toThrow(
            /reserved/
        )
    })

    it('rejects the reserved name http-redirect', () => {
        expect(() =>
            validatePorts([feature('acme', [{ name: 'http-redirect', port: 1234 }])])
        ).toThrow(/reserved/)
    })

    it('rejects port 80, which the supervisor binds for its own redirect listener', () => {
        expect(() => validatePorts([feature('acme', [{ name: 'acme-sync', port: 80 }])])).toThrow(
            /80.*supervisor|supervisor.*80/
        )
    })

    it('rejects port 443, which the supervisor binds for its own HTTPS listener', () => {
        expect(() => validatePorts([feature('acme', [{ name: 'acme-sync', port: 443 }])])).toThrow(
            /443.*supervisor|supervisor.*443/
        )
    })

    it('rejects port 7090, which the supervisor binds for its own default HTTP listener', () => {
        expect(() => validatePorts([feature('acme', [{ name: 'acme-sync', port: 7090 }])])).toThrow(
            /7090.*supervisor|supervisor.*7090/
        )
    })
})
