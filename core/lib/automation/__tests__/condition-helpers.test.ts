import { describe, expect, it } from 'vitest'
import type { CatalogField } from '../api'
import {
    addCondition,
    addGroup,
    ensureUids,
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
