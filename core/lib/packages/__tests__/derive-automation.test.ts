import { describe, expect, it } from 'vitest'
import { deriveAutomation } from '../derive-automation'

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
        const catalog = deriveAutomation([] as never)
        expect(catalog.byPackage[0].pkgSlug).toBe('core')
        expect(catalog.triggers['core:schedule']).toBeDefined()
        expect(catalog.triggers['core:manual']).toBeDefined()
        expect(catalog.actions['core:apply-label']).toBeDefined()
        expect(catalog.actions['core:notify']).toBeDefined()
    })

    it('keys package declarations by qualified ref and skips packages without automation', () => {
        const catalog = deriveAutomation([mailEntry, plainEntry] as never)
        expect(catalog.triggers['mail:message-received']).toMatchObject({
            pkgSlug: 'mail',
            pkgName: 'Mail',
        })
        expect(catalog.byPackage.map(p => p.pkgSlug)).toEqual(['core', 'mail'])
    })
})
