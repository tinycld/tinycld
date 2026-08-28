import { describe, expect, it } from 'vitest'
import type { CatalogResponse } from '../../lib/automation/api'
import { draftToRecord, emptyDraft, validateDraft } from '../../lib/automation/draft'

// The IF card renders a ready-to-fill first condition row WITHOUT putting it in
// the draft (ConditionsCard's SyntheticFirstGroup). These pin the two halves of
// why that has to stay render-only:
//
//   • validateDraft must not error on a rule the user never added conditions to
//     — a seeded blank row would report "Condition field '' is not available".
//   • draftToRecord must not throw — conditionSchema.field is min(1) and
//     conditionGroupSchema.conditions is min(1), so a blank condition or an
//     empty group makes conditionsAstSchema.parse throw ON SAVE, turning a UX
//     nicety into a crash.
//
// Nothing else covers draft.ts, so a future "just seed it in emptyDraft, it's
// simpler" refactor would otherwise land silently.

const CATALOG: CatalogResponse = {
    triggers: [
        {
            ref: 'mail:message-received',
            pkg: 'mail',
            id: 'message-received',
            label: 'A message arrives',
            collection: 'mail_messages',
            fields: [
                { key: 'subject', label: 'Subject', type: 'text' },
                { key: 'size', label: 'Size', type: 'number' },
            ],
        },
        {
            ref: 'core:manual',
            pkg: 'core',
            id: 'manual',
            label: 'Run manually',
            synthetic: 'manual',
            fields: [],
        },
    ],
    actions: [
        {
            ref: 'core:notify',
            pkg: 'core',
            id: 'notify',
            label: 'Send me a notification',
            kind: 'native',
            available: true,
            params: [{ key: 'title', label: 'Title', template: true, field: { type: 'text' } }],
        },
    ],
} as unknown as CatalogResponse

function savableDraft() {
    return {
        ...emptyDraft('personal'),
        name: 'My rule',
        trigger: 'mail:message-received',
        actions: [{ uid: 'a1', ref: 'core:notify', params: { title: 'hi' } }],
    }
}

describe('validateDraft with no conditions', () => {
    it('accepts a rule whose condition row was never filled in', () => {
        // The exact state the builder is in when a user picks a trigger, adds an
        // action, and saves without touching the offered condition row.
        expect(validateDraft(savableDraft(), CATALOG)).toEqual([])
    })

    it('still requires a name, a trigger and an action', () => {
        const errors = validateDraft(emptyDraft('personal'), CATALOG)
        expect(errors).toContain('Name is required')
        expect(errors).toContain('Trigger is required')
        expect(errors).toContain('At least one action is required')
    })
})

describe('validateDraft with a partly-filled condition', () => {
    it('rejects a numeric condition with no value', () => {
        // Field chosen but value left blank is INCOMPLETE, not blank: the user
        // showed intent by picking a field, and the engine's numeric operators
        // fail closed on an unreadable value, so saving it would save a rule
        // that silently never matches.
        const draft = savableDraft()
        draft.conditions = {
            match: 'all',
            groups: [{ uid: 'g1', match: 'all', conditions: [{ field: 'size', op: 'gt' }] }],
        }
        expect(validateDraft(draft, CATALOG)).toContain("Condition on 'Size' needs a number")
    })

    it('rejects a condition naming a field the trigger does not expose', () => {
        const draft = savableDraft()
        draft.conditions = {
            match: 'all',
            groups: [
                {
                    uid: 'g1',
                    match: 'all',
                    conditions: [{ field: 'nope', op: 'contains', value: 'x' }],
                },
            ],
        }
        expect(validateDraft(draft, CATALOG)).toContain(
            "Condition field 'nope' is not available on trigger 'A message arrives'"
        )
    })
})

describe('validateDraft on a synthetic trigger', () => {
    it('rejects conditions on a trigger with no record behind it', () => {
        // The IF card renders disabled for schedule/manual rather than hiding,
        // and this is the rule that disabling is enforcing.
        const draft = { ...savableDraft(), trigger: 'core:manual' }
        draft.conditions = {
            match: 'all',
            groups: [
                {
                    uid: 'g1',
                    match: 'all',
                    conditions: [{ field: 'subject', op: 'contains', value: 'x' }],
                },
            ],
        }
        expect(validateDraft(draft, CATALOG)).toContain(
            "Trigger 'Run manually' has no fields — it cannot have conditions"
        )
    })
})

describe('draftToRecord', () => {
    it('serializes an untouched condition set without throwing', () => {
        const record = draftToRecord(savableDraft())
        expect(record.conditions).toEqual({ match: 'all', groups: [] })
    })

    it('throws on a blank condition — which is why none is ever seeded', () => {
        // Documents the failure mode the render-only approach avoids. If this
        // ever stops throwing, the schemas were loosened and the constraint
        // driving SyntheticFirstGroup's design no longer holds.
        const draft = savableDraft()
        draft.conditions = {
            match: 'all',
            groups: [{ uid: 'g1', match: 'all', conditions: [{ field: '', op: '' }] }],
        }
        expect(() => draftToRecord(draft)).toThrow()
    })

    it('throws on an empty group — likewise', () => {
        const draft = savableDraft()
        draft.conditions = {
            match: 'all',
            groups: [{ uid: 'g1', match: 'all', conditions: [] }],
        }
        expect(() => draftToRecord(draft)).toThrow()
    })

    it('strips the builder-local uid from persisted conditions', () => {
        const draft = savableDraft()
        draft.conditions = {
            match: 'all',
            groups: [
                {
                    uid: 'g1',
                    match: 'all',
                    conditions: [{ uid: 'c1', field: 'subject', op: 'contains', value: 'invoice' }],
                },
            ],
        }
        const record = draftToRecord(draft)
        expect(record.conditions).toEqual({
            match: 'all',
            groups: [
                {
                    match: 'all',
                    conditions: [{ field: 'subject', op: 'contains', value: 'invoice' }],
                },
            ],
        })
    })
})
