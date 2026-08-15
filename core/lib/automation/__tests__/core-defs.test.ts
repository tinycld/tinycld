import { describe, expect, it } from 'vitest'
import { CORE_AUTOMATION, CORE_PKG_SLUG } from '../core-defs'
import { validateDefinitions } from '../schemas'

describe('CORE_AUTOMATION', () => {
    it('is a valid definition set (synthetic triggers allowed for core)', () => {
        expect(
            validateDefinitions(CORE_PKG_SLUG, CORE_AUTOMATION, { allowSynthetic: true })
        ).toEqual([])
    })

    it('declares the built-in triggers and actions from the spec', () => {
        const triggerIds = (CORE_AUTOMATION.triggers ?? []).map(t => t.id)
        const actionIds = (CORE_AUTOMATION.actions ?? []).map(a => a.id)
        expect(triggerIds).toEqual(['schedule', 'manual', 'user-added'])
        expect(actionIds).toEqual(['apply-label', 'notify', 'send-email'])
    })

    // users carries password and tokenKey. The engine's exposure rules filter
    // both out everywhere regardless, but this trigger's allowlist must never
    // name them in the first place — the filter is a backstop, not the
    // statement of intent.
    it('user-added exposes no credential columns', () => {
        const userAdded = (CORE_AUTOMATION.triggers ?? []).find(t => t.id === 'user-added')
        expect(userAdded).toMatchObject({ collection: 'users', on: 'create' })

        const fieldKeys = (
            (userAdded as { fields?: (string | { key: string })[] }).fields ?? []
        ).map(f => (typeof f === 'string' ? f : f.key))
        expect(fieldKeys).toEqual(['name', 'username', 'email', 'role'])
        for (const secret of ['password', 'tokenKey', 'metadata']) {
            expect(fieldKeys).not.toContain(secret)
        }
    })

    it('apply-label writes a polymorphic label_assignments record from context', () => {
        const applyLabel = (CORE_AUTOMATION.actions ?? []).find(a => a.id === 'apply-label')
        expect(applyLabel).toMatchObject({
            kind: 'record-op',
            collection: 'label_assignments',
            op: {
                type: 'create',
                set: {
                    label: { param: 'label' },
                    record_id: { context: 'record-id' },
                    collection: { context: 'collection' },
                    user: { context: 'owner' },
                },
            },
        })
    })
})
