import { describe, expect, it } from 'vitest'
import { deriveAutomation } from '../derive-automation'
import type { PackageManifest } from '../types'

const mailEntry = {
    manifest: { name: 'Mail', slug: 'mail' },
    automation: {
        triggers: [
            {
                id: 'message-received',
                label: 'A message arrives',
                collection: 'mail_messages',
                on: 'create' as const,
            },
        ],
        actions: [],
    },
}
const plainEntry = { manifest: { name: 'Calc', slug: 'calc' } }

describe('deriveAutomation', () => {
    it('always includes the core catalog, first', () => {
        const catalog = deriveAutomation([])
        expect(catalog.byPackage[0].pkgSlug).toBe('core')
        expect(catalog.triggers['core:schedule']).toBeDefined()
        expect(catalog.triggers['core:manual']).toBeDefined()
        expect(catalog.actions['core:apply-label']).toBeDefined()
        expect(catalog.actions['core:notify']).toBeDefined()
    })

    it('keys package declarations by qualified ref and skips packages without automation', () => {
        const catalog = deriveAutomation([mailEntry, plainEntry])
        expect(catalog.triggers['mail:message-received']).toMatchObject({
            pkgSlug: 'mail',
            pkgName: 'Mail',
        })
        expect(catalog.byPackage.map(p => p.pkgSlug)).toEqual(['core', 'mail'])
    })
})

describe('PackageManifest automation field shape', () => {
    it('PackageManifest carries the raw automation pointer shape', () => {
        const manifest = {
            name: 'X',
            slug: 'x',
            version: '0.0.1',
            description: 'd',
            automation: { definitions: 'automation' },
        } satisfies PackageManifest
        expect(manifest.automation.definitions).toBe('automation')
    })
})
