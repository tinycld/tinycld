import { describe, expect, it } from 'vitest'
import {
    aggregatesFor,
    columnHint,
    defaultAggregate,
    describePath,
    nextColumnId,
    pathLabel,
    sourceAtLinks,
} from '../columns'
import { CATALOG, SALES_KEY, TEAMS_KEY } from './catalog-fixture'

describe('describePath', () => {
    it('follows a to-one link to a field', () => {
        const info = describePath(CATALOG, SALES_KEY, [{ link: 'team' }, { field: 'name' }])
        expect(info && pathLabel(info)).toBe('Team › Name')
        expect(info).toMatchObject({ viaToMany: false, mode: undefined })
    })

    it('marks a path through a to-many link and keeps its mode', () => {
        const info = describePath(CATALOG, TEAMS_KEY, [
            { link: 'sales', mode: 'aggregate' },
            { field: 'amount' },
        ])
        expect(info).toMatchObject({ viaToMany: true, mode: 'aggregate' })
        expect(info?.field.key).toBe('amount')
    })

    it('reads an unknown link or field as nothing', () => {
        expect(describePath(CATALOG, SALES_KEY, [{ link: 'vendor' }, { field: 'name' }])).toBeNull()
        expect(describePath(CATALOG, SALES_KEY, [{ field: 'gone' }])).toBeNull()
        expect(describePath(CATALOG, 'zoo.gone', [{ field: 'name' }])).toBeNull()
    })

    it('finds the source a list of links reaches', () => {
        expect(sourceAtLinks(CATALOG, SALES_KEY, [{ link: 'team' }, { link: 'sales' }])?.key).toBe(
            SALES_KEY
        )
        expect(sourceAtLinks(CATALOG, SALES_KEY, [{ link: 'nope' }])).toBeNull()
    })
})

describe('aggregates', () => {
    const sales = CATALOG.get(SALES_KEY)
    const amount = sales?.fields.find(f => f.key === 'amount')
    const day = sales?.fields.find(f => f.key === 'day')

    it('offers a field its own aggregates on its own source', () => {
        expect(amount && aggregatesFor(amount, false)).toEqual([
            'sum',
            'avg',
            'min',
            'max',
            'count',
            'countDistinct',
        ])
        expect(amount && defaultAggregate(amount, false)).toBe('')
    })

    it('refuses what a to-many total cannot add up', () => {
        // An amount with a per-row currency cannot be totalled per parent, so only a count.
        expect(amount && aggregatesFor(amount, true)).toEqual(['count'])
        expect(amount && defaultAggregate(amount, true)).toBe('count')
        expect(day && aggregatesFor(day, true)).toEqual(['min', 'max', 'count'])
    })
})

describe('nextColumnId', () => {
    // A formula reads columns by id, and the forms formula editor accepts only
    // lowercase snake_case keys of at most 40 characters.
    const KEY = /^[a-z][a-z0-9_]{0,39}$/

    it('makes an id from a hint and keeps it unique', () => {
        expect(nextColumnId([], 'customer')).toBe('customer')
        expect(nextColumnId(['customer'], 'customer')).toBe('customer2')
        expect(nextColumnId(['customer', 'customer2'], 'customer')).toBe('customer3')
        expect(nextColumnId([], '1 st/name')).toBe('st_name')
        expect(nextColumnId([], '42')).toBe('col')
    })

    it('makes a lowercase snake_case id of at most 40 characters', () => {
        expect(nextColumnId([], 'teamName')).toBe('team_name')
        expect(nextColumnId([], 'Net  Amount!')).toBe('net_amount')
        const long = 'a'.repeat(50)
        expect(nextColumnId([], long)).toBe('a'.repeat(40))
        const twice = nextColumnId(['a'.repeat(40)], long)
        expect(twice).toBe(`${'a'.repeat(39)}2`)
        for (const id of [twice, nextColumnId([], 'teamName'), nextColumnId([], '1 st/name')]) {
            expect(id).toMatch(KEY)
        }
    })

    it('hints an id from a path', () => {
        expect(columnHint([{ link: 'team' }, { field: 'name' }])).toBe('team_name')
        expect(columnHint([{ field: 'amount' }])).toBe('amount')
        expect(columnHint([{ link: 'salesLines' }, { field: 'unitPrice' }])).toBe(
            'sales_lines_unit_price'
        )
    })
})
