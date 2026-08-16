import { describe, expect, it } from 'vitest'
import { conditionsAstSchema, ruleActionsSchema, validateDefinitions } from '../schemas'
import type { AutomationDefinitions } from '../types'

const validAst = {
    match: 'all',
    groups: [
        {
            match: 'any',
            conditions: [
                { field: 'sender_email', op: 'contains', value: '@acme.com' },
                { field: 'sender_email', op: 'contains', value: '@example.com' },
            ],
        },
        { match: 'all', conditions: [{ field: 'has_attachments', op: 'is_true' }] },
    ],
}

describe('conditionsAstSchema', () => {
    it('accepts the spec example AST', () => {
        expect(conditionsAstSchema.safeParse(validAst).success).toBe(true)
    })

    it('accepts an empty groups array (rule with no conditions)', () => {
        expect(conditionsAstSchema.safeParse({ match: 'all', groups: [] }).success).toBe(true)
    })

    it('rejects an unknown operator', () => {
        const bad = {
            match: 'all',
            groups: [{ match: 'any', conditions: [{ field: 'x', op: 'regex', value: 'y' }] }],
        }
        expect(conditionsAstSchema.safeParse(bad).success).toBe(false)
    })

    it('rejects a value-carrying op with no value', () => {
        const bad = {
            match: 'all',
            groups: [{ match: 'any', conditions: [{ field: 'x', op: 'contains' }] }],
        }
        expect(conditionsAstSchema.safeParse(bad).success).toBe(false)
    })

    it('accepts a value-less op with no value', () => {
        const ok = {
            match: 'all',
            groups: [{ match: 'any', conditions: [{ field: 'x', op: 'is_empty' }] }],
        }
        expect(conditionsAstSchema.safeParse(ok).success).toBe(true)
    })

    it('rejects a value-less op that carries a stray value', () => {
        const bad = {
            match: 'all',
            groups: [{ match: 'any', conditions: [{ field: 'x', op: 'is_empty', value: 'junk' }] }],
        }
        expect(conditionsAstSchema.safeParse(bad).success).toBe(false)
    })

    it('rejects a group with zero conditions', () => {
        const bad = { match: 'all', groups: [{ match: 'any', conditions: [] }] }
        expect(conditionsAstSchema.safeParse(bad).success).toBe(false)
    })
})

describe('ruleActionsSchema', () => {
    it('accepts an ordered action list with qualified refs', () => {
        const ok = [
            { ref: 'mail:move-to-folder', params: { folder: 'abc123def456ghi' } },
            { ref: 'core:apply-label', params: { label: 'abc123def456ghi' } },
        ]
        expect(ruleActionsSchema.safeParse(ok).success).toBe(true)
    })

    it('rejects an unqualified ref and an empty list', () => {
        expect(ruleActionsSchema.safeParse([{ ref: 'move-to-folder', params: {} }]).success).toBe(
            false
        )
        expect(ruleActionsSchema.safeParse([]).success).toBe(false)
    })

    it('defaults params to an empty object', () => {
        const parsed = ruleActionsSchema.parse([{ ref: 'core:notify' }])
        expect(parsed[0].params).toEqual({})
    })
})

describe('validateDefinitions', () => {
    const good: AutomationDefinitions = {
        triggers: [
            {
                id: 'message-received',
                label: 'A message arrives',
                collection: 'mail_messages',
                on: 'create',
            },
        ],
        actions: [
            {
                id: 'move-to-folder',
                label: 'Move to folder',
                kind: 'record-op',
                collection: 'mail_messages',
                op: {
                    type: 'update',
                    target: 'trigger-record',
                    set: { alias: { param: 'alias' } },
                },
                params: [{ key: 'alias', field: 'alias' }],
            },
        ],
    }

    it('returns no errors for a valid definition set', () => {
        expect(validateDefinitions('mail', good)).toEqual([])
    })

    it('rejects malformed and duplicate ids', () => {
        const dup: AutomationDefinitions = {
            triggers: [
                { id: 'Same_Id', label: 'x', collection: 'c', on: 'create' },
                { id: 'Same_Id', label: 'y', collection: 'c', on: 'create' },
            ],
        }
        const errors = validateDefinitions('mail', dup)
        expect(errors.some(e => e.includes('kebab-case'))).toBe(true)
        expect(errors.some(e => e.includes('duplicate'))).toBe(true)
    })

    it('rejects synthetic triggers outside core', () => {
        const synthetic: AutomationDefinitions = {
            triggers: [{ id: 'schedule', label: 'On a schedule', synthetic: 'schedule' }],
        }
        expect(validateDefinitions('mail', synthetic).some(e => e.includes('synthetic'))).toBe(true)
        expect(validateDefinitions('core', synthetic, { allowSynthetic: true })).toEqual([])
    })

    it('rejects a watch clause on a non-update trigger', () => {
        const bad: AutomationDefinitions = {
            triggers: [
                {
                    id: 'thing-created',
                    label: 'A thing is created',
                    collection: 'things',
                    on: 'create',
                    watch: ['status'],
                },
            ],
        }
        expect(validateDefinitions('mail', bad).some(e => e.includes('declares watch'))).toBe(true)
    })

    it('rejects an empty fields list on a trigger', () => {
        const bad: AutomationDefinitions = {
            triggers: [
                {
                    id: 'message-received',
                    label: 'A message arrives',
                    collection: 'mail_messages',
                    on: 'create',
                    fields: [],
                },
            ],
        }
        expect(validateDefinitions('mail', bad).some(e => e.includes('empty fields list'))).toBe(
            true
        )
    })

    it('rejects a record-op whose set references an undeclared param', () => {
        const bad: AutomationDefinitions = {
            actions: [
                {
                    id: 'move',
                    label: 'Move',
                    kind: 'record-op',
                    collection: 'c',
                    op: {
                        type: 'update',
                        target: 'trigger-record',
                        set: { folder: { param: 'missing' } },
                    },
                    params: [],
                },
            ],
        }
        expect(validateDefinitions('mail', bad).some(e => e.includes('missing'))).toBe(true)
    })

    // A typed relation param with no target reaches the UI as a picker over
    // nothing — the failure that stayed silent until runtime before this rule.
    it('rejects a typed relation param without a relationTarget', () => {
        const bad: AutomationDefinitions = {
            actions: [
                {
                    id: 'add-assignee',
                    label: 'Add an assignee',
                    kind: 'native',
                    params: [{ key: 'user', type: 'relation' }],
                },
            ],
        }
        expect(validateDefinitions('cards', bad).some(e => e.includes('no relationTarget'))).toBe(
            true
        )
    })

    it('rejects a relationTarget on a non-relation typed param', () => {
        const bad: AutomationDefinitions = {
            actions: [
                {
                    id: 'add-assignee',
                    label: 'Add an assignee',
                    kind: 'native',
                    params: [{ key: 'note', type: 'text', relationTarget: 'users' }],
                },
            ],
        }
        expect(
            validateDefinitions('cards', bad).some(e => e.includes('declares relationTarget'))
        ).toBe(true)
    })

    it('accepts a typed relation param that names its target', () => {
        const good: AutomationDefinitions = {
            actions: [
                {
                    id: 'add-assignee',
                    label: 'Add an assignee',
                    kind: 'native',
                    params: [{ key: 'user', type: 'relation', relationTarget: 'users' }],
                },
            ],
        }
        expect(validateDefinitions('cards', good)).toEqual([])
    })

    // Column params inherit the column's target; declaring one is not the
    // authoring mistake the typed-param rule guards against.
    it('leaves column-referencing params out of the relation checks', () => {
        const good: AutomationDefinitions = {
            actions: [
                {
                    id: 'move',
                    label: 'Move',
                    kind: 'record-op',
                    collection: 'c',
                    op: {
                        type: 'update',
                        target: 'trigger-record',
                        set: { folder: { param: 'folder' } },
                    },
                    params: [{ key: 'folder', field: 'folder' }],
                },
            ],
        }
        expect(validateDefinitions('mail', good)).toEqual([])
    })
})
