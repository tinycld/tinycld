// @vitest-environment happy-dom

import { act, renderHook, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { CatalogResponse } from '../api'
import type { RuleDraft } from '../draft'
import { emptyDraft } from '../draft'
import { useRuleDraft } from '../use-rule-draft'

const catalog: CatalogResponse = {
    triggers: [
        { ref: 'core:manual', pkg: 'core', label: 'Run manually', synthetic: 'manual' },
        {
            ref: 'cat:item-created',
            pkg: 'cat',
            label: 'An item is created',
            collection: 'cat_items',
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
                    required: true,
                },
            ],
        },
        { ref: 'core:ping', pkg: 'core', label: 'Ping', kind: 'native', available: true },
        {
            ref: 'core:retired',
            pkg: 'core',
            label: 'Retired',
            kind: 'native',
            available: false,
            params: [
                {
                    key: 'title',
                    label: 'Title',
                    field: { key: 'title', label: 'Title', type: 'text' },
                    template: false,
                    required: true,
                },
            ],
        },
    ],
}

function renderDraft(initial: RuleDraft = emptyDraft('personal')) {
    return renderHook(() => useRuleDraft(initial, catalog))
}

async function save(result: { current: ReturnType<typeof useRuleDraft> }, onSave = vi.fn()) {
    await act(async () => {
        await result.current.handleSubmit(onSave)()
    })
    return onSave
}

describe('useRuleDraft', () => {
    it('blocks Save on an empty draft and files each error on its field', async () => {
        const { result } = renderDraft()
        const onSave = await save(result)

        expect(onSave).not.toHaveBeenCalled()
        expect(result.current.isSubmitted).toBe(true)
        expect(result.current.errors.name?.message).toBe('Name is required')
        expect(result.current.errors.trigger?.message).toBe('Trigger is required')
        expect(result.current.errors.actions?.message).toBe('At least one action is required')
    })

    it('saves the parsed draft, with the name trimmed', async () => {
        const { result } = renderDraft({
            ...emptyDraft('personal'),
            name: '  My rule  ',
            trigger: 'core:manual',
        })
        act(() => result.current.addAction('core:ping'))
        const onSave = await save(result)

        expect(onSave).toHaveBeenCalledTimes(1)
        const saved = onSave.mock.calls[0][0] as RuleDraft
        expect(saved.name).toBe('My rule')
        expect(saved.actions.map(a => a.ref)).toEqual(['core:ping'])
    })

    it('resets conditions and actions when the trigger changes, but not on a re-pick', () => {
        const { result } = renderDraft({ ...emptyDraft('personal'), trigger: 'cat:item-created' })
        act(() => result.current.addAction('core:ping'))
        act(() =>
            result.current.setConditions({
                match: 'all',
                groups: [
                    {
                        uid: 'g1',
                        match: 'all',
                        conditions: [{ uid: 'c1', field: 'subject', op: 'contains', value: 'x' }],
                    },
                ],
            })
        )

        act(() => result.current.selectTrigger('cat:item-created'))
        expect(result.current.draft.actions).toHaveLength(1)
        expect(result.current.draft.conditions.groups).toHaveLength(1)

        act(() => result.current.selectTrigger('core:manual'))
        expect(result.current.draft.trigger).toBe('core:manual')
        expect(result.current.draft.actions).toEqual([])
        expect(result.current.actionRows).toEqual([])
        expect(result.current.draft.conditions).toEqual({ match: 'all', groups: [] })
    })

    it('keeps each action row key with its action across a reorder and a removal', () => {
        const { result } = renderDraft({ ...emptyDraft('personal'), trigger: 'core:manual' })
        act(() => result.current.addAction('core:notify'))
        act(() => result.current.addAction('core:ping'))
        const [notifyKey, pingKey] = result.current.actionRows.map(r => r.key)

        act(() => result.current.moveAction(0, 1))
        expect(result.current.actionRows.map(r => [r.key, r.action.ref])).toEqual([
            [pingKey, 'core:ping'],
            [notifyKey, 'core:notify'],
        ])

        // Out of range is ignored rather than clamped.
        act(() => result.current.moveAction(1, 2))
        expect(result.current.actionRows.map(r => r.key)).toEqual([pingKey, notifyKey])

        act(() => result.current.removeAction(0))
        expect(result.current.actionRows.map(r => [r.key, r.action.ref])).toEqual([
            [notifyKey, 'core:notify'],
        ])
    })

    it('shows param edits in the action rows and re-validates them after a failed Save', async () => {
        const { result } = renderDraft({
            ...emptyDraft('personal'),
            name: 'My rule',
            trigger: 'core:manual',
        })
        act(() => result.current.addAction('core:notify'))
        await save(result)
        expect(result.current.errors.actions?.[0]?.params?.title?.message).toBe(
            "Action 'Notify' requires 'Title'"
        )

        act(() => result.current.setActionParams(0, { title: 'Hello' }))
        expect(result.current.actionRows[0].action.params).toEqual({ title: 'Hello' })
        await waitFor(() => expect(result.current.errors.actions).toBeUndefined())
    })

    it('keeps an action-level error and its param errors side by side', async () => {
        const { result } = renderDraft({
            ...emptyDraft('personal'),
            name: 'My rule',
            trigger: 'core:manual',
        })
        act(() => result.current.addAction('core:retired'))
        await save(result)

        const actionError = result.current.errors.actions?.[0]
        expect(actionError?.message).toBe("Action 'Retired' is not available")
        expect(actionError?.params?.title?.message).toBe("Action 'Retired' requires 'Title'")
    })
})

describe('setCatalog', () => {
    const withAnswer: CatalogResponse = {
        ...catalog,
        triggers: catalog.triggers.map(t =>
            t.ref === 'cat:item-created'
                ? {
                      ...t,
                      fields: [
                          ...(t.fields ?? []),
                          { key: 'fields.answer', label: 'Answer', type: 'text' },
                      ],
                  }
                : t
        ),
    }
    const initial: RuleDraft = {
        ...emptyDraft('org'),
        name: 'r',
        trigger: 'cat:item-created',
        conditions: {
            match: 'all',
            groups: [
                {
                    uid: 'g1',
                    match: 'all',
                    conditions: [{ uid: 'c1', field: 'fields.answer', op: 'contains', value: 'x' }],
                },
            ],
        },
        actions: [{ uid: 'a1', ref: 'core:ping', params: {} }],
    }

    it('validates against the catalog given in the latest render', async () => {
        const { result, rerender } = renderHook(
            ({ c }) => {
                const form = useRuleDraft(initial, catalog)
                form.setCatalog(c)
                return form
            },
            { initialProps: { c: catalog } }
        )
        const refused = await save(result)
        expect(refused).not.toHaveBeenCalled()

        rerender({ c: withAnswer })
        const accepted = await save(result)
        expect(accepted).toHaveBeenCalled()
    })
})
