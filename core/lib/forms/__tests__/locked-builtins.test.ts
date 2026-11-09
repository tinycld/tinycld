import { describe, expect, it } from 'vitest'
import { type FormDefinition, validateDefinitionForHost } from '../definition'
import { defaultDefinitionFor, hostDefSchema } from '../host'

/** `terms` is optional but locked: the record may move and relabel it, never hide it. */
const host = hostDefSchema.parse({
    key: 'gizmos:bill',
    label: 'Bill',
    collection: 'gizmos_bills',
    formColumn: 'form',
    fieldsColumn: 'fields',
    columns: ['name', 'terms', 'form', 'fields'],
    builtin: [
        { key: 'name', label: 'Name', kind: 'text', storage: 'column', required: true },
        { key: 'terms', label: 'Terms', kind: 'text', storage: 'column', locked: true },
    ],
})

const base = defaultDefinitionFor(host)
const twoPages: FormDefinition = {
    ...base,
    pages: [...base.pages, { key: 'more', title: 'More' }],
}

interface TermsChange {
    hidden?: boolean
    page?: string
    label?: string
}

function withTerms({ hidden, page, label }: TermsChange): FormDefinition {
    return {
        ...twoPages,
        fields: twoPages.fields.map(f =>
            f.key === 'terms'
                ? {
                      ...f,
                      hidden: hidden ?? f.hidden,
                      page: page ?? f.page,
                      label: label ?? f.label,
                  }
                : f
        ),
    }
}

function hiding(action: { action: 'hide'; field: string } | { action: 'hide'; page: string }) {
    return {
        ...twoPages,
        fields: twoPages.fields.map(f => (f.key === 'terms' ? { ...f, page: 'more' } : f)),
        rules: [
            {
                key: 'r',
                match: 'all' as const,
                when: [{ field: 'name', op: 'is_empty' as const }],
                actions: [action],
            },
        ],
    }
}

describe('validateDefinitionForHost, locked built-ins', () => {
    it('refuses a definition that hides a locked built-in', () => {
        expect(validateDefinitionForHost(withTerms({ hidden: true }), host)).toEqual([
            {
                path: ['fields', 1, 'hidden'],
                message: 'terms is locked by the record and cannot be hidden',
            },
        ])
    })

    it('refuses a rule that hides a locked built-in', () => {
        expect(validateDefinitionForHost(hiding({ action: 'hide', field: 'terms' }), host)).toEqual(
            [
                {
                    path: ['rules', 0, 'actions', 0, 'field'],
                    message: 'cannot hide locked field terms',
                },
            ]
        )
    })

    it('refuses a rule that hides the page holding a locked built-in', () => {
        expect(validateDefinitionForHost(hiding({ action: 'hide', page: 'more' }), host)).toEqual([
            {
                path: ['rules', 0, 'actions', 0, 'page'],
                message: 'page more holds locked field terms',
            },
        ])
    })

    it('accepts a locked built-in moved to another page and relabeled', () => {
        expect(
            validateDefinitionForHost(withTerms({ page: 'more', label: 'Payment terms' }), host)
        ).toEqual([])
    })
})
