import type { Rules } from '@tinycld/core/types/pbSchema'
import { describe, expect, it } from 'vitest'
import type { CatalogResponse } from '../api'
import type { RuleDraft } from '../draft'
import { draftToRecord, emptyDraft, recordToDraft, validateDraft } from '../draft'

// Mirrors Task 1's catalog fixture shape (see api.test.ts / catalog_test.go).
const catalog: CatalogResponse = {
    triggers: [
        { ref: 'core:manual', pkg: 'core', label: 'Run manually', synthetic: 'manual' },
        { ref: 'core:schedule', pkg: 'core', label: 'On a schedule', synthetic: 'schedule' },
        {
            ref: 'cat:item-created',
            pkg: 'cat',
            label: 'An item is created',
            collection: 'cat_items',
            fields: [
                { key: 'subject', label: 'Subject', type: 'text' },
                { key: 'has_attachments', label: 'Has attachments', type: 'boolean' },
                {
                    key: 'status',
                    label: 'Status',
                    type: 'select',
                    options: ['new', 'done'],
                },
                {
                    key: 'folder',
                    label: 'Folder',
                    type: 'relation',
                    relationTarget: 'cat_folders',
                    displayField: 'name',
                },
            ],
        },
        {
            ref: 'cat:other-created',
            pkg: 'cat',
            label: 'An other-thing is created',
            collection: 'cat_others',
            fields: [{ key: 'subject', label: 'Subject', type: 'text' }],
        },
    ],
    actions: [
        {
            ref: 'core:notify',
            pkg: 'core',
            label: 'Notify',
            kind: 'native',
            available: true,
            params: [
                {
                    key: 'title',
                    label: 'Title',
                    field: { key: 'title', label: 'Title', type: 'text' },
                    template: true,
                },
            ],
        },
        {
            ref: 'core:unavailable-action',
            pkg: 'core',
            label: 'Unavailable',
            kind: 'native',
            available: false,
            params: [],
        },
        {
            ref: 'cat:set-folder',
            pkg: 'cat',
            label: 'Move to folder',
            kind: 'record-op',
            collection: 'cat_items',
            opType: 'update',
            opTarget: 'trigger-record',
            available: true,
            params: [
                {
                    key: 'folder',
                    label: 'Folder',
                    field: {
                        key: 'folder',
                        label: 'Folder',
                        type: 'relation',
                        relationTarget: 'cat_folders',
                        displayField: 'name',
                    },
                    template: false,
                },
            ],
        },
        {
            ref: 'cat:other-set-folder',
            pkg: 'cat',
            label: 'Move other to folder (different collection)',
            kind: 'record-op',
            collection: 'cat_others',
            opType: 'update',
            opTarget: 'trigger-record',
            available: true,
            params: [
                {
                    key: 'folder',
                    label: 'Folder',
                    field: {
                        key: 'folder',
                        label: 'Folder',
                        type: 'relation',
                        relationTarget: 'cat_folders',
                        displayField: 'name',
                    },
                    template: false,
                },
            ],
        },
    ],
}

function baseRecord(overrides: Partial<Rules> = {}): Rules {
    return {
        id: 'rule123456789012',
        name: 'My rule',
        scope: 'personal',
        owner: 'user123456789012',
        trigger: 'cat:item-created',
        trigger_config: {},
        conditions: { match: 'all', groups: [] },
        actions: [{ ref: 'core:notify', params: { title: 'hi' } }],
        enabled: true,
        order: 0,
        stop_processing: false,
        created: '2026-01-01 00:00:00.000Z',
        updated: '2026-01-01 00:00:00.000Z',
        ...overrides,
    }
}

describe('emptyDraft', () => {
    it('has the requested scope and produces the expected messages', () => {
        const draft = emptyDraft('org')
        expect(draft.scope).toBe('org')
        expect(draft.id).toBeUndefined()
        expect(draft.name).toBe('')
        expect(draft.trigger).toBe('')
        expect(draft.conditions).toEqual({ match: 'all', groups: [] })
        expect(draft.actions).toEqual([])
        expect(draft.enabled).toBe(true)
        expect(draft.stopProcessing).toBe(false)
        expect(draft.order).toBe(0)

        const errors = validateDraft(draft, catalog)
        expect(errors.some(e => /name/i.test(e))).toBe(true)
        expect(errors.some(e => /trigger/i.test(e))).toBe(true)
        expect(errors.some(e => /action/i.test(e))).toBe(true)
    })
})

describe('record <-> draft round-trip', () => {
    it('preserves all fields', () => {
        const record = baseRecord({
            trigger_config: { cron: '0 * * * *' },
            actions: [
                { ref: 'core:notify', params: { title: 'hi', urgent: true, count: 3 } },
                { ref: 'cat:set-folder', params: { folder: 'folder123456789' } },
            ],
            stop_processing: true,
            order: 5,
        })

        const draft = recordToDraft(record)
        expect(draft).toEqual<RuleDraft>({
            id: record.id,
            name: record.name,
            scope: record.scope,
            trigger: record.trigger,
            triggerConfig: { cron: '0 * * * *' },
            conditions: { match: 'all', groups: [] },
            actions: [
                { ref: 'core:notify', params: { title: 'hi', urgent: true, count: 3 } },
                { ref: 'cat:set-folder', params: { folder: 'folder123456789' } },
            ],
            enabled: record.enabled,
            stopProcessing: true,
            order: 5,
        })

        const fields = draftToRecord(draft)
        expect(fields).toEqual({
            name: record.name,
            scope: record.scope,
            trigger: record.trigger,
            trigger_config: { cron: '0 * * * *' },
            conditions: { match: 'all', groups: [] },
            actions: draft.actions,
            enabled: record.enabled,
            stop_processing: true,
            order: 5,
        })
    })

    it('draftToRecord omits id/owner/created/updated', () => {
        const draft = recordToDraft(baseRecord())
        const fields = draftToRecord(draft)
        expect(fields).not.toHaveProperty('id')
        expect(fields).not.toHaveProperty('owner')
        expect(fields).not.toHaveProperty('created')
        expect(fields).not.toHaveProperty('updated')
    })
})

describe('recordToDraft tolerance', () => {
    it('degrades malformed conditions JSON to an empty AST, no throw', () => {
        const record = baseRecord({ conditions: 'not-an-ast' as unknown as Rules['conditions'] })
        expect(() => recordToDraft(record)).not.toThrow()
        const draft = recordToDraft(record)
        expect(draft.conditions).toEqual({ match: 'all', groups: [] })
    })

    it('degrades malformed actions JSON to an empty actions list, no throw', () => {
        const record = baseRecord({ actions: { not: 'an array' } as unknown as Rules['actions'] })
        expect(() => recordToDraft(record)).not.toThrow()
        const draft = recordToDraft(record)
        expect(draft.actions).toEqual([])
    })

    it('degrades a null trigger_config to an empty object, no throw', () => {
        const record = baseRecord({ trigger_config: null as unknown as Rules['trigger_config'] })
        expect(() => recordToDraft(record)).not.toThrow()
        const draft = recordToDraft(record)
        expect(draft.triggerConfig).toEqual({})
    })
})

describe('validateDraft', () => {
    function validDraft(overrides: Partial<RuleDraft> = {}): RuleDraft {
        return {
            name: 'My rule',
            scope: 'personal',
            trigger: 'cat:item-created',
            triggerConfig: {},
            conditions: { match: 'all', groups: [] },
            actions: [{ ref: 'core:notify', params: { title: 'hi' } }],
            enabled: true,
            stopProcessing: false,
            order: 0,
            ...overrides,
        }
    }

    it('accepts a valid draft', () => {
        expect(validateDraft(validDraft(), catalog)).toEqual([])
    })

    it('returns no catalog-dependent errors when catalog is undefined, still requires name/trigger/actions', () => {
        const errors = validateDraft(emptyDraft('personal'), undefined)
        expect(errors.length).toBeGreaterThan(0)
    })

    it('requires a name', () => {
        const errors = validateDraft(validDraft({ name: '  ' }), catalog)
        expect(errors.some(e => /name/i.test(e))).toBe(true)
    })

    it('rejects an unknown trigger', () => {
        const errors = validateDraft(validDraft({ trigger: 'cat:does-not-exist' }), catalog)
        expect(errors.some(e => /trigger/i.test(e))).toBe(true)
    })

    it('rejects a condition field not present in the trigger catalog', () => {
        const draft = validDraft({
            conditions: {
                match: 'all',
                groups: [
                    {
                        match: 'all',
                        conditions: [{ field: 'not-a-field', op: 'contains', value: 'x' }],
                    },
                ],
            },
        })
        const errors = validateDraft(draft, catalog)
        expect(errors.some(e => /not-a-field/.test(e))).toBe(true)
    })

    it('rejects an operator illegal for the field type', () => {
        const draft = validDraft({
            conditions: {
                match: 'all',
                groups: [
                    {
                        match: 'all',
                        conditions: [{ field: 'has_attachments', op: 'contains', value: 'x' }],
                    },
                ],
            },
        })
        const errors = validateDraft(draft, catalog)
        expect(errors.some(e => /operator/i.test(e))).toBe(true)
    })

    it('requires at least one action', () => {
        const errors = validateDraft(validDraft({ actions: [] }), catalog)
        expect(errors.some(e => /action/i.test(e))).toBe(true)
    })

    it('rejects an action ref not in the catalog', () => {
        const errors = validateDraft(
            validDraft({ actions: [{ ref: 'cat:does-not-exist', params: {} }] }),
            catalog
        )
        expect(errors.some(e => /does-not-exist/.test(e))).toBe(true)
    })

    it('rejects an unavailable action', () => {
        const errors = validateDraft(
            validDraft({ actions: [{ ref: 'core:unavailable-action', params: {} }] }),
            catalog
        )
        expect(errors.some(e => /unavailable/i.test(e) || /available/i.test(e))).toBe(true)
    })

    it('rejects a trigger-record action targeting a different collection than the trigger', () => {
        // trigger is cat:item-created (collection cat_items); action targets cat_others
        const errors = validateDraft(
            validDraft({
                actions: [{ ref: 'cat:other-set-folder', params: { folder: 'folder123456789' } }],
            }),
            catalog
        )
        expect(errors.some(e => /collection/i.test(e))).toBe(true)
    })

    it('rejects a synthetic-triggered rule that has conditions', () => {
        const draft = validDraft({
            trigger: 'core:manual',
            conditions: {
                match: 'all',
                groups: [
                    { match: 'all', conditions: [{ field: 'x', op: 'contains', value: 'y' }] },
                ],
            },
            actions: [{ ref: 'core:notify', params: { title: 'hi' } }],
        })
        const errors = validateDraft(draft, catalog)
        expect(errors.some(e => /condition/i.test(e))).toBe(true)
    })

    it('rejects a synthetic-triggered rule with a trigger-record action', () => {
        const draft = validDraft({
            trigger: 'core:manual',
            conditions: { match: 'all', groups: [] },
            actions: [{ ref: 'cat:set-folder', params: { folder: 'folder123456789' } }],
        })
        const errors = validateDraft(draft, catalog)
        expect(errors.some(e => /trigger-record|synthetic/i.test(e))).toBe(true)
    })

    it('requires a non-empty cron for the schedule trigger', () => {
        const draft = validDraft({
            trigger: 'core:schedule',
            triggerConfig: {},
            actions: [{ ref: 'core:notify', params: { title: 'hi' } }],
        })
        const errors = validateDraft(draft, catalog)
        expect(errors.some(e => /cron/i.test(e))).toBe(true)
    })

    it('accepts the schedule trigger with a cron string', () => {
        const draft = validDraft({
            trigger: 'core:schedule',
            triggerConfig: { cron: '0 * * * *' },
            actions: [{ ref: 'core:notify', params: { title: 'hi' } }],
        })
        expect(validateDraft(draft, catalog)).toEqual([])
    })

    it('rejects a relation param left empty', () => {
        const errors = validateDraft(
            validDraft({ actions: [{ ref: 'cat:set-folder', params: { folder: '' } }] }),
            catalog
        )
        expect(errors.length).toBeGreaterThan(0)
    })

    it('allows a text param to be an empty string', () => {
        const errors = validateDraft(
            validDraft({ actions: [{ ref: 'core:notify', params: { title: '' } }] }),
            catalog
        )
        expect(errors).toEqual([])
    })

    it('allows a missing param key on a text param (treated as empty)', () => {
        const errors = validateDraft(
            validDraft({ actions: [{ ref: 'core:notify', params: {} }] }),
            catalog
        )
        expect(errors).toEqual([])
    })
})
