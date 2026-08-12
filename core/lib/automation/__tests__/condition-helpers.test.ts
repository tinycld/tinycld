import { describe, expect, it } from 'vitest'
import type { CatalogField } from '../api'
import {
    addCondition,
    addGroup,
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
    it('appends an empty all-match group', () => {
        const next = addGroup(EMPTY_AST)
        expect(next.groups).toHaveLength(1)
        expect(next.groups[0]).toEqual({ match: 'all', conditions: [] })
        // immutability: source untouched
        expect(EMPTY_AST.groups).toHaveLength(0)
    })

    it('removes the group at the given index', () => {
        const withTwo = addGroup(addGroup(EMPTY_AST))
        const next = removeGroup(withTwo, 0)
        expect(next.groups).toHaveLength(1)
        expect(withTwo.groups).toHaveLength(2)
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
    it('adds an empty condition to the target group only', () => {
        const ast = addGroup(addGroup(EMPTY_AST))
        const next = addCondition(ast, 0)
        expect(next.groups[0].conditions).toEqual([{ field: '', op: '', value: undefined }])
        expect(next.groups[1].conditions).toHaveLength(0)
        expect(ast.groups[0].conditions).toHaveLength(0)
    })

    it('updates a condition immutably by [group, condition] index', () => {
        const ast = addCondition(addGroup(EMPTY_AST), 0)
        const next = updateCondition(ast, 0, 0, { field: 'subject', op: 'contains' })
        expect(next.groups[0].conditions[0]).toEqual({
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
        const next = removeCondition(ast, 0, 0)
        expect(next.groups[0].conditions).toEqual([{ field: 'b', op: '', value: undefined }])
    })
})
