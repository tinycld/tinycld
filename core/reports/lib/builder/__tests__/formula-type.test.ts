import { describe, expect, it } from 'vitest'
import { formulaType, lookupPicks, typesOf } from '../formula-type'
import { CATALOG, SALES_KEY, TEAMS_KEY } from './catalog-fixture'
import { CTX, LARGER, LOOKUP, SHARE, WITH_PERIOD, WITH_SECTIONS } from './formula-fixture'

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

    it('types a bound from its param, as a param is typed', () => {
        expect(formulaType({ param: 'period', bound: 'to' }, types)).toBe('date')
        expect(formulaType({ param: 'gone', bound: 'from' }, types)).toBe('')
        expect(formulaType({ param: 'statuses', bound: 'from' }, types)).toBe('text')
    })

    // lookup.go lookupIn: pick sum adds numbers or amounts, and refuses any other value.
    it('offers Add up only on a lookup whose value is a number or an amount', () => {
        const lookupOf = (field: string) => ({
            op: 'lookup',
            source: SALES_KEY,
            value: { path: [{ field }] },
            pick: 'min' as const,
        })
        const picksOf = (node: typeof LOOKUP) => lookupPicks(node, types).map(p => p.value)
        expect(picksOf(lookupOf('amount'))).toEqual(['min', 'max', 'sum', 'count'])
        expect(picksOf(lookupOf('quantity'))).toEqual(['min', 'max', 'sum', 'count'])
        expect(picksOf(lookupOf('customer'))).toEqual(['min', 'max', 'count'])
        expect(picksOf(lookupOf('day'))).toEqual(['min', 'max', 'count'])
        expect(picksOf(LOOKUP)).toEqual(['min', 'max', 'count'])
    })

    // A pick read back on a value Add up refuses stays in the menu, so it shows selected.
    it('keeps Add up in the menu while it is the pick', () => {
        const stale = {
            op: 'lookup',
            source: SALES_KEY,
            value: { path: [{ field: 'customer' }] },
            pick: 'sum' as const,
        }
        expect(lookupPicks(stale, types).map(p => p.value)).toEqual(['min', 'max', 'sum', 'count'])
    })
})

describe('a percent of a section', () => {
    it('is a percent', () => {
        expect(formulaType(SHARE, typesOf(WITH_SECTIONS))).toBe('percent')
    })
})
