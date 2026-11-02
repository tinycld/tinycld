import { describe, expect, it } from 'vitest'
import type { SpecHop } from '../../spec'
import type { Aggregate } from '../../types'
import {
    aggregatesFor,
    columnHint,
    defaultAggregate,
    describePath,
    firstTextField,
    linkTarget,
    nextColumnId,
    pathLabel,
    sourceAtLinks,
    switchableModeHop,
    withHopJoin,
    withHopMode,
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

    it('reads a path that ends at a to-one link as a relation to its target', () => {
        const teams = CATALOG.get(TEAMS_KEY)
        expect(describePath(CATALOG, SALES_KEY, [{ link: 'team' }])).toEqual({
            field: {
                key: 'id',
                label: 'Team',
                type: 'relation',
                aggregates: ['count', 'countDistinct'],
            },
            source: teams,
            target: teams,
            viaToMany: false,
            mode: undefined,
            labels: ['Team'],
        })
        expect(describePath(CATALOG, TEAMS_KEY, [{ link: 'sales' }])).toBeNull()
        // Only the last link must be to-one: a total column may cross a to-many link first.
        const viaSales = describePath(CATALOG, TEAMS_KEY, [
            { link: 'sales', mode: 'aggregate' },
            { link: 'team' },
        ])
        expect(viaSales).toMatchObject({ viaToMany: true, mode: 'aggregate', target: teams })
        expect(viaSales?.field).toMatchObject({ key: 'id', label: 'Team', type: 'relation' })
        expect(describePath(CATALOG, SALES_KEY, [{ field: 'id' }])?.target?.key).toBe(SALES_KEY)
        expect(describePath(CATALOG, SALES_KEY, [{ field: 'amount' }])?.target).toBeUndefined()
    })

    it('names a record by its first text field', () => {
        const teams = CATALOG.get(TEAMS_KEY)
        expect(teams && firstTextField(teams)?.key).toBe('name')
    })

    it('finds the source a list of links reaches', () => {
        expect(sourceAtLinks(CATALOG, SALES_KEY, [{ link: 'team' }, { link: 'sales' }])?.key).toBe(
            SALES_KEY
        )
        expect(sourceAtLinks(CATALOG, SALES_KEY, [{ link: 'nope' }])).toBeNull()
    })
})

describe('linkTarget', () => {
    const column = (path: SpecHop[], aggregate: Aggregate | '' = '') => ({
        kind: 'path' as const,
        path,
        aggregate,
    })

    it('reads the source a link column points at', () => {
        expect(linkTarget(CATALOG, SALES_KEY, column([{ link: 'team' }]))?.key).toBe(TEAMS_KEY)
        expect(linkTarget(CATALOG, SALES_KEY, column([{ field: 'id' }]))?.key).toBe(SALES_KEY)
    })

    it('reads no target for a field, a total or a formula', () => {
        expect(linkTarget(CATALOG, SALES_KEY, column([{ field: 'customer' }]))).toBeUndefined()
        expect(linkTarget(CATALOG, SALES_KEY, column([{ link: 'team' }], 'count'))).toBeUndefined()
        expect(linkTarget(CATALOG, SALES_KEY, { kind: 'formula', aggregate: '' })).toBeUndefined()
    })
})

describe('hop joins and modes', () => {
    it('requires a link, and keeps rows without it again, keeping the mode', () => {
        const hops: SpecHop[] = [{ link: 'sales', mode: 'aggregate' }, { field: 'quantity' }]
        const required = withHopJoin(hops, 0, true)
        expect(required).toEqual([
            { link: 'sales', mode: 'aggregate', with: 'required' },
            { field: 'quantity' },
        ])
        expect(withHopJoin(required, 0, false)).toEqual(hops)
        // A field step has no join.
        expect(withHopJoin(hops, 1, true)).toEqual(hops)
        expect(hops[0]).toEqual({ link: 'sales', mode: 'aggregate' })
    })

    it('turns a total into child rows, which have no join', () => {
        const hops: SpecHop[] = [
            { link: 'sales', mode: 'aggregate', with: 'required' },
            { field: 'quantity' },
        ]
        expect(withHopMode(hops, 0, 'nest')).toEqual([
            { link: 'sales', mode: 'nest' },
            { field: 'quantity' },
        ])
        expect(withHopMode([{ link: 'sales', mode: 'nest' }], 0, 'aggregate')).toEqual([
            { link: 'sales', mode: 'aggregate' },
        ])
    })

    it('lets the mode switch only on a path that crosses one to-many link', () => {
        const once = [{ link: 'sales', mode: 'aggregate' as const }, { field: 'quantity' }]
        expect(switchableModeHop(CATALOG, TEAMS_KEY, once)).toBe(0)
        expect(switchableModeHop(CATALOG, SALES_KEY, [{ link: 'team' }, { field: 'name' }])).toBe(
            null
        )
        const twice = [
            { link: 'sales', mode: 'nest' as const },
            { link: 'team' },
            { link: 'sales', mode: 'nest' as const },
            { field: 'quantity' },
        ]
        expect(switchableModeHop(CATALOG, TEAMS_KEY, twice)).toBeNull()
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
