import { describe, expect, it } from 'vitest'
import type { CatalogAction, CatalogField, CatalogTrigger } from '../api'
import type { ActionItem } from '../condition-helpers'
import {
    addCondition,
    addGroup,
    appendPlaceholder,
    compatibleActions,
    ensureUids,
    mergeReorderedSubset,
    moveAction,
    operatorLabel,
    operatorsForField,
    removeCondition,
    removeGroup,
    setGroupMatch,
    setTopMatch,
    updateCondition,
} from '../condition-helpers'
import { ALL_OPS } from '../helpers'

function field(type: CatalogField['type']): CatalogField {
    return { key: 'f', label: 'F', type }
}

describe('operatorsForField', () => {
    it('excludes is_empty for number fields', () => {
        expect(operatorsForField(field('number'))).toEqual(['eq', 'neq', 'gt', 'lt'])
    })

    it('returns is/is_not for select fields', () => {
        expect(operatorsForField(field('select'))).toEqual(['is', 'is_not'])
    })

    it('returns [] for an unknown field type', () => {
        expect(
            operatorsForField({ key: 'f', label: 'F', type: 'bogus' as CatalogField['type'] })
        ).toEqual([])
    })

    it('does not mutate the source OPERATORS_BY_TYPE entry', () => {
        operatorsForField(field('number'))
        expect(operatorsForField(field('relation'))).toContain('is_empty')
    })
})

describe('operatorLabel', () => {
    it('covers every op in ALL_OPS with an explicit human label (never the snake_case fallback)', () => {
        for (const op of ALL_OPS) {
            const label = operatorLabel(op)
            expect(label).toBeTruthy()
            if (op.includes('_')) expect(label).not.toBe(op)
        }
    })

    it('falls back to the raw op string for an unknown op', () => {
        expect(operatorLabel('totally_unknown_op')).toBe('totally_unknown_op')
    })
})

const EMPTY_AST = { match: 'all' as const, groups: [] }

describe('addGroup / removeGroup', () => {
    it('appends an empty all-match group with a generated uid', () => {
        const next = addGroup(EMPTY_AST)
        expect(next.groups).toHaveLength(1)
        expect(next.groups[0]).toMatchObject({ match: 'all', conditions: [] })
        expect(next.groups[0].uid).toBeTruthy()
        // immutability: source untouched
        expect(EMPTY_AST.groups).toHaveLength(0)
    })

    it('removes the group at the given index', () => {
        const withTwo = addGroup(addGroup(EMPTY_AST))
        const next = removeGroup(withTwo, 0)
        expect(next.groups).toHaveLength(1)
        expect(withTwo.groups).toHaveLength(2)
    })

    it('gives each added group a distinct uid', () => {
        const ast = addGroup(addGroup(EMPTY_AST))
        expect(ast.groups[0].uid).not.toBe(ast.groups[1].uid)
    })
})

describe('setGroupMatch / setTopMatch', () => {
    it('sets a single group match mode without touching others', () => {
        const ast = addGroup(addGroup(EMPTY_AST))
        const next = setGroupMatch(ast, 1, 'any')
        expect(next.groups[0].match).toBe('all')
        expect(next.groups[1].match).toBe('any')
        expect(ast.groups[1].match).toBe('all')
    })

    it('sets the top-level match mode', () => {
        const next = setTopMatch(EMPTY_AST, 'any')
        expect(next.match).toBe('any')
        expect(EMPTY_AST.match).toBe('all')
    })
})

describe('addCondition / removeCondition / updateCondition', () => {
    it('adds an empty condition with a generated uid to the target group only', () => {
        const ast = addGroup(addGroup(EMPTY_AST))
        const next = addCondition(ast, 0)
        expect(next.groups[0].conditions).toMatchObject([{ field: '', op: '', value: undefined }])
        expect(next.groups[0].conditions[0].uid).toBeTruthy()
        expect(next.groups[1].conditions).toHaveLength(0)
        expect(ast.groups[0].conditions).toHaveLength(0)
    })

    it('gives each added condition a distinct uid', () => {
        let ast = addGroup(EMPTY_AST)
        ast = addCondition(ast, 0)
        ast = addCondition(ast, 0)
        expect(ast.groups[0].conditions[0].uid).not.toBe(ast.groups[0].conditions[1].uid)
    })

    it('updates a condition immutably by [group, condition] index, preserving its uid', () => {
        const ast = addCondition(addGroup(EMPTY_AST), 0)
        const uid = ast.groups[0].conditions[0].uid
        const next = updateCondition(ast, 0, 0, { field: 'subject', op: 'contains' })
        expect(next.groups[0].conditions[0]).toEqual({
            uid,
            field: 'subject',
            op: 'contains',
            value: undefined,
        })
        expect(ast.groups[0].conditions[0].field).toBe('')
    })

    it('removes a condition by index, leaving siblings intact', () => {
        let ast = addGroup(EMPTY_AST)
        ast = addCondition(ast, 0)
        ast = addCondition(ast, 0)
        ast = updateCondition(ast, 0, 0, { field: 'a' })
        ast = updateCondition(ast, 0, 1, { field: 'b' })
        const survivorUid = ast.groups[0].conditions[1].uid
        const next = removeCondition(ast, 0, 0)
        expect(next.groups[0].conditions).toEqual([
            { uid: survivorUid, field: 'b', op: '', value: undefined },
        ])
    })
})

describe('ensureUids', () => {
    it('is a no-op for an already-uid-bearing AST', () => {
        const ast = addCondition(addGroup(EMPTY_AST), 0)
        expect(ensureUids(ast)).toEqual(ast)
    })

    it('assigns uids to groups/conditions loaded without one (e.g. straight off the wire)', () => {
        const wireAst = {
            match: 'all' as const,
            groups: [
                {
                    match: 'all' as const,
                    conditions: [{ field: 'subject', op: 'contains', value: 'x' }],
                },
            ],
        }
        const next = ensureUids(wireAst)
        expect(next.groups[0].uid).toBeTruthy()
        expect(next.groups[0].conditions[0].uid).toBeTruthy()
        // semantic content untouched
        expect(next.groups[0].match).toBe('all')
        expect(next.groups[0].conditions[0]).toMatchObject({
            field: 'subject',
            op: 'contains',
            value: 'x',
        })
    })

    it('is a no-op on an empty AST', () => {
        expect(ensureUids(EMPTY_AST)).toEqual(EMPTY_AST)
    })
})

// Mirrors draft.test.ts's catalog fixture shape (see api.test.ts / catalog_test.go).
const recordTrigger: CatalogTrigger = {
    ref: 'cat:item-created',
    pkg: 'cat',
    label: 'An item is created',
    collection: 'cat_items',
    fields: [{ key: 'subject', label: 'Subject', type: 'text' }],
}
const otherCollectionTrigger: CatalogTrigger = {
    ref: 'cat:other-created',
    pkg: 'cat',
    label: 'An other-thing is created',
    collection: 'cat_others',
    fields: [{ key: 'subject', label: 'Subject', type: 'text' }],
}
const syntheticTrigger: CatalogTrigger = {
    ref: 'core:manual',
    pkg: 'core',
    label: 'Run manually',
    synthetic: 'manual',
}

const nativeAction: CatalogAction = {
    ref: 'core:notify',
    pkg: 'core',
    label: 'Notify',
    kind: 'native',
    available: true,
}
const unavailableAction: CatalogAction = {
    ref: 'core:unavailable-action',
    pkg: 'core',
    label: 'Unavailable',
    kind: 'native',
    available: false,
}
const createOpAction: CatalogAction = {
    ref: 'cat:create-item',
    pkg: 'cat',
    label: 'Create item',
    kind: 'record-op',
    collection: 'cat_items',
    opType: 'create',
    available: true,
}
const triggerRecordOpAction: CatalogAction = {
    ref: 'cat:set-folder',
    pkg: 'cat',
    label: 'Move to folder',
    kind: 'record-op',
    collection: 'cat_items',
    opType: 'update',
    opTarget: 'trigger-record',
    available: true,
}
const otherTriggerRecordOpAction: CatalogAction = {
    ref: 'cat:other-set-folder',
    pkg: 'cat',
    label: 'Move other to folder (different collection)',
    kind: 'record-op',
    collection: 'cat_others',
    opType: 'update',
    opTarget: 'trigger-record',
    available: true,
}

describe('compatibleActions', () => {
    const allActions = [
        nativeAction,
        unavailableAction,
        createOpAction,
        triggerRecordOpAction,
        otherTriggerRecordOpAction,
    ]
    const catalog = { triggers: [recordTrigger], actions: allActions }

    it('includes trigger-record ops only when their collection matches the trigger', () => {
        const refs = compatibleActions(catalog, recordTrigger).map(a => a.ref)
        expect(refs).toContain(triggerRecordOpAction.ref)
        expect(refs).not.toContain(otherTriggerRecordOpAction.ref)
    })

    it('includes the matching trigger-record op for a different trigger collection', () => {
        const catalogForOther = { triggers: [otherCollectionTrigger], actions: allActions }
        const refs = compatibleActions(catalogForOther, otherCollectionTrigger).map(a => a.ref)
        expect(refs).toContain(otherTriggerRecordOpAction.ref)
        expect(refs).not.toContain(triggerRecordOpAction.ref)
    })

    it('always includes create-op record actions and native actions', () => {
        const refs = compatibleActions(catalog, recordTrigger).map(a => a.ref)
        expect(refs).toContain(createOpAction.ref)
        expect(refs).toContain(nativeAction.ref)
    })

    it('excludes all trigger-record ops for a synthetic trigger', () => {
        const refs = compatibleActions(catalog, syntheticTrigger).map(a => a.ref)
        expect(refs).not.toContain(triggerRecordOpAction.ref)
        expect(refs).not.toContain(otherTriggerRecordOpAction.ref)
        // create-op and native actions remain available even with no trigger record
        expect(refs).toContain(createOpAction.ref)
        expect(refs).toContain(nativeAction.ref)
    })

    it('keeps unavailable actions listed with available:false rather than filtering them out', () => {
        const result = compatibleActions(catalog, recordTrigger)
        const unavailable = result.find(a => a.ref === unavailableAction.ref)
        expect(unavailable).toBeDefined()
        expect(unavailable?.available).toBe(false)
    })
})

describe('moveAction', () => {
    it('swaps an item earlier in the list', () => {
        expect(moveAction(['a', 'b', 'c'], 1, 0)).toEqual(['b', 'a', 'c'])
    })

    it('swaps an item later in the list', () => {
        expect(moveAction(['a', 'b', 'c'], 0, 2)).toEqual(['b', 'c', 'a'])
    })

    it('is a no-op when the destination is out of bounds (below zero)', () => {
        const list = ['a', 'b', 'c']
        expect(moveAction(list, 0, -1)).toEqual(list)
    })

    it('is a no-op when the destination is out of bounds (past the end)', () => {
        const list = ['a', 'b', 'c']
        expect(moveAction(list, 2, 3)).toEqual(list)
    })

    it('does not mutate the source array', () => {
        const list = ['a', 'b', 'c']
        moveAction(list, 0, 1)
        expect(list).toEqual(['a', 'b', 'c'])
    })

    it('preserves each action object (and its uid) across a reorder — moves whole objects, not just values', () => {
        const actions: ActionItem[] = [
            { uid: 'uid-a', ref: 'core:notify', params: {} },
            { uid: 'uid-b', ref: 'core:notify', params: {} },
            { uid: 'uid-c', ref: 'core:notify', params: {} },
        ]
        const next = moveAction(actions, 0, 2)
        expect(next.map(a => a.uid)).toEqual(['uid-b', 'uid-c', 'uid-a'])
        // Each entry is the exact same object reference, not a rebuilt copy —
        // so ActionEntry's key={draftAction.uid} really does travel with its
        // action's identity, not just a copied uid string.
        expect(next[2]).toBe(actions[0])
    })
})

describe('appendPlaceholder', () => {
    it('appends a {{key}} placeholder to an empty value', () => {
        expect(appendPlaceholder('', 'subject')).toBe('{{subject}}')
    })

    it('appends to existing text without a separating space', () => {
        expect(appendPlaceholder('Re: ', 'subject')).toBe('Re: {{subject}}')
    })

    it('appends after existing text that has no trailing space', () => {
        expect(appendPlaceholder('Hello', 'name')).toBe('Hello{{name}}')
    })
})

describe('mergeReorderedSubset', () => {
    it('keeps ids outside the subset fixed while the subset scattered through the list reorders in place', () => {
        // Full list: a b c d e — subset {b, d} (mail-filtered rows), reversed
        // to d then b. a/c/e (non-mail rows) must stay exactly where they were.
        const full = ['a', 'b', 'c', 'd', 'e']
        const reorderedSubset = ['d', 'b']
        expect(mergeReorderedSubset(full, reorderedSubset)).toEqual(['a', 'd', 'c', 'b', 'e'])
    })

    it('is identity-compatible when the subset is the full list (no pkgFilter case)', () => {
        const full = ['a', 'b', 'c']
        const reordered = ['c', 'a', 'b']
        expect(mergeReorderedSubset(full, reordered)).toEqual(reordered)
    })

    it('is a true no-op when the subset order is unchanged', () => {
        const full = ['a', 'b', 'c', 'd']
        const subset = ['b', 'd']
        expect(mergeReorderedSubset(full, subset)).toEqual(full)
    })

    it('appends ids present in the subset but absent from the full list, defensively', () => {
        const full = ['a', 'b']
        const reorderedSubset = ['b', 'a', 'z']
        expect(mergeReorderedSubset(full, reorderedSubset)).toEqual(['b', 'a', 'z'])
    })
})
