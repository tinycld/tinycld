import { describe, expect, it } from 'vitest'

import type { CatalogResponse, ConditionsAst } from '../api'
import { conditionsAstSchema } from '../schemas'

describe('CatalogResponse types', () => {
    it('accepts a representative catalog response literal', () => {
        const response: CatalogResponse = {
            triggers: [
                {
                    ref: 'core:manual',
                    pkg: 'core',
                    label: 'Run manually',
                    synthetic: 'manual',
                },
                {
                    ref: 'cat:item-created',
                    pkg: 'cat',
                    label: 'An item is created',
                    collection: 'cat_items',
                    fields: [
                        {
                            key: 'subject',
                            label: 'Subject',
                            type: 'text',
                        },
                        {
                            key: 'has_attachments',
                            label: 'Has attachments',
                            type: 'boolean',
                        },
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
            ],
            actions: [
                {
                    ref: 'core:notify',
                    pkg: 'core',
                    label: 'Notify',
                    kind: 'native',
                    available: false,
                    params: [
                        {
                            key: 'title',
                            label: 'Title',
                            field: {
                                key: 'title',
                                label: 'Title',
                                type: 'text',
                            },
                            template: true,
                        },
                    ],
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
            ],
        }

        // Verify type-checking passes; this is a compile-time assertion
        expect(response).toBeTruthy()
    })
})

describe('ConditionsAst type inference', () => {
    it('round-trips through conditionsAstSchema.parse', () => {
        const ast: ConditionsAst = {
            match: 'all',
            groups: [
                {
                    match: 'any',
                    conditions: [{ field: 'sender_email', op: 'contains', value: '@acme.com' }],
                },
            ],
        }

        const parsed = conditionsAstSchema.parse(ast)
        expect(parsed).toEqual(ast)
    })

    it('accepts the builder default empty groups', () => {
        const ast: ConditionsAst = {
            match: 'all',
            groups: [],
        }

        const parsed = conditionsAstSchema.parse(ast)
        expect(parsed).toEqual(ast)
    })
})
