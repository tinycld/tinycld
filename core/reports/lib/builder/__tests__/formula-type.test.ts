import { describe, expect, it } from 'vitest'
import { formulaType, typesOf } from '../formula-type'
import { CATALOG, SALES_KEY, TEAMS_KEY } from './catalog-fixture'
import { CTX, LARGER, LOOKUP, WITH_PERIOD } from './formula-fixture'

describe('formulaType', () => {
    const types = {
        catalog: CATALOG,
        source: SALES_KEY,
        col: (k: string) => CTX.values.find(v => v.key === k)?.type,
        records: CTX.records,
        params: WITH_PERIOD.params,
    }

    it('types a formula as the engine does', () => {
        expect(formulaType(LARGER, types)).toBe('money')
        const amountByQty = { op: 'div', args: [{ col: 'amount' }, { col: 'qty' }], scale: 0 }
        expect(formulaType(amountByQty, types)).toBe('money')
        expect(formulaType({ op: 'gt', args: [{ col: 'qty' }, { value: 1 }] }, types)).toBe('bool')
        const count = { op: 'count', args: [{ path: [{ field: 'customer' }] }] }
        expect(formulaType(count, types)).toBe('number')
        expect(formulaType({ op: 'concat', args: [{ value: 'a' }] }, types)).toBe('text')
        expect(formulaType(LOOKUP, types)).toBe('relation')
        expect(formulaType({ path: [{ link: 'team' }] }, types)).toBe('relation')
    })
    it('types sums, averages and params by the engine rules', () => {
        const pct = { op: 'percentOfTotal', args: [{ col: 'qty' }], scale: 2 }
        expect(formulaType({ op: 'add', args: [pct, { value: 1 }] }, types)).toBe('percent')
        expect(formulaType({ op: 'avg', args: [pct], scale: 2 }, types)).toBe('number')
        expect(formulaType({ op: 'avg', args: [{ col: 'amount' }], scale: 0 }, types)).toBe('money')
        expect(formulaType({ param: 'statuses' }, types)).toBe('text')
        expect(formulaType({ value: [1, 2] }, types)).toBe('number')
    })
    it('skips empty results, shows a select mixed with text as text, and gives no type it cannot know', () => {
        const day = { path: [{ field: 'day' }] }
        expect(formulaType({ op: 'coalesce', args: [{ value: null }, day] }, types)).toBe('date')
        const allEmpty = { op: 'if', args: [{ value: true }, { value: null }, { value: null }] }
        expect(formulaType(allEmpty, types)).toBe('')
        const nested = { op: 'if', args: [{ value: true }, allEmpty, day] }
        expect(formulaType(nested, types)).toBe('date')
        const status = { path: [{ field: 'status' }] }
        expect(formulaType({ op: 'if', args: [{ value: true }, status, status] }, types)).toBe(
            'select'
        )
        const mixed = {
            op: 'case',
            args: [{ value: true }, status, { path: [{ field: 'customer' }] }],
        }
        expect(formulaType(mixed, types)).toBe('text')
        expect(formulaType({ op: 'shout', args: [] }, types)).toBe('')
        expect(formulaType({ value: null }, types)).toBe('')
    })
    it('reads the types a context offers', () => {
        expect(typesOf(CTX).col('qty')).toBe('number')
    })
})

describe('formulaType of the v3 parts', () => {
    const types = {
        catalog: CATALOG,
        source: SALES_KEY,
        rowSource: SALES_KEY,
        col: (k: string) => CTX.values.find(v => v.key === k)?.type,
        records: CTX.records,
        params: WITH_PERIOD.params,
    }

    it('types a count as a number, a sum as its value, a bound as a date', () => {
        expect(formulaType({ ...LOOKUP, pick: 'count' }, types)).toBe('number')
        const added = {
            op: 'lookup',
            source: SALES_KEY,
            value: { path: [{ field: 'amount' }] },
            pick: 'sum' as const,
        }
        expect(formulaType(added, types)).toBe('money')
        expect(formulaType({ param: 'period', bound: 'from' }, types)).toBe('date')
    })

    it("types This row's field from the report's source inside a lookup on another source", () => {
        const day = {
            op: 'lookup',
            source: TEAMS_KEY,
            value: { outer: [{ field: 'day' }] },
        }
        expect(formulaType(day, types)).toBe('date')
        expect(typesOf(CTX).rowSource).toBe(SALES_KEY)
    })
})
