import { needsPackage, ruleSummary } from '@tinycld/core/components/rules/rule-summary'
import type { Rules } from '@tinycld/core/types/pbSchema'
import { describe, expect, it } from 'vitest'
import type { CatalogResponse } from '../api'

// Mirrors the fixture shape established in draft.test.ts.
const catalog: CatalogResponse = {
    triggers: [
        { ref: 'core:manual', pkg: 'core', label: 'Run manually', synthetic: 'manual' },
        {
            ref: 'mail:message-arrived',
            pkg: 'mail',
            label: 'A message arrives',
            collection: 'mail_messages',
            fields: [{ key: 'subject', label: 'Subject', type: 'text' }],
        },
    ],
    actions: [
        { ref: 'core:notify', pkg: 'core', label: 'Notify', kind: 'native', available: true },
        {
            ref: 'mail:move-folder',
            pkg: 'mail',
            label: 'Move to folder',
            kind: 'record-op',
            available: true,
        },
        {
            ref: 'mail:apply-label',
            pkg: 'mail',
            label: 'Apply label',
            kind: 'record-op',
            available: true,
        },
    ],
}

function makeRule(overrides: Partial<Rules> = {}): Rules {
    return {
        id: 'r1',
        name: 'Test rule',
        scope: 'personal',
        owner: 'u1',
        trigger: 'mail:message-arrived',
        trigger_config: {},
        conditions: { match: 'all', groups: [] },
        actions: [],
        enabled: true,
        order: 0,
        stop_processing: false,
        created: '',
        updated: '',
        ...overrides,
    }
}

describe('ruleSummary', () => {
    it('composes trigger label, condition count, and action labels', () => {
        const rule = makeRule({
            conditions: {
                match: 'all',
                groups: [
                    {
                        match: 'all',
                        conditions: [{ field: 'subject', op: 'contains', value: 'x' }],
                    },
                    {
                        match: 'all',
                        conditions: [{ field: 'subject', op: 'contains', value: 'y' }],
                    },
                ],
            },
            actions: [
                { ref: 'mail:move-folder', params: {} },
                { ref: 'mail:apply-label', params: {} },
            ],
        })
        expect(ruleSummary(rule, catalog)).toBe(
            'A message arrives · 2 conditions · Move to folder, Apply label'
        )
    })

    it('singularizes a single condition', () => {
        const rule = makeRule({
            conditions: {
                match: 'all',
                groups: [
                    {
                        match: 'all',
                        conditions: [{ field: 'subject', op: 'contains', value: 'x' }],
                    },
                ],
            },
            actions: [{ ref: 'core:notify', params: {} }],
        })
        expect(ruleSummary(rule, catalog)).toBe('A message arrives · 1 condition · Notify')
    })

    it('omits the condition segment when there are none', () => {
        const rule = makeRule({ actions: [{ ref: 'core:notify', params: {} }] })
        expect(ruleSummary(rule, catalog)).toBe('A message arrives · Notify')
    })

    it('joins multiple action labels', () => {
        const rule = makeRule({
            actions: [
                { ref: 'mail:move-folder', params: {} },
                { ref: 'mail:apply-label', params: {} },
            ],
        })
        expect(ruleSummary(rule, catalog)).toBe('A message arrives · Move to folder, Apply label')
    })

    it('falls back to the raw ref for a trigger/action missing from the catalog', () => {
        const rule = makeRule({
            trigger: 'unknown:trigger',
            actions: [{ ref: 'unknown:action', params: {} }],
        })
        expect(ruleSummary(rule, catalog)).toBe('unknown:trigger · unknown:action')
    })

    it('handles a synthetic manual trigger with no conditions', () => {
        const rule = makeRule({
            trigger: 'core:manual',
            actions: [{ ref: 'core:notify', params: {} }],
        })
        expect(ruleSummary(rule, catalog)).toBe('Run manually · Notify')
    })
})

describe('needsPackage', () => {
    it('returns null when the trigger resolves in the catalog', () => {
        const rule = makeRule()
        expect(needsPackage(rule, catalog)).toBeNull()
    })

    it('returns the missing package slug when the trigger is not in the catalog', () => {
        const rule = makeRule({ trigger: 'calendar:event-created' })
        expect(needsPackage(rule, catalog)).toBe('calendar')
    })

    it('returns null for a malformed ref rather than throwing', () => {
        const rule = makeRule({ trigger: 'not-a-ref' })
        expect(needsPackage(rule, catalog)).toBeNull()
    })
})
