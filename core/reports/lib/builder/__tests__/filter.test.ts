import { describe, expect, it } from 'vitest'
import type { SpecNode } from '../../spec'
import type { CatalogField } from '../../types'
import {
    aboveOf,
    askHighParamFor,
    askParamFor,
    type ConditionDraft,
    conditionProblems,
    emptyCondition,
    filterToNode,
    type GroupDraft,
    type LevelScope,
    nodeToFilter,
    opsFor,
    opsShownFor,
    paramsInGroup,
    withOptionsGroup,
} from '../filter'
import { CATALOG, SALES_KEY, TEAMS_KEY } from './catalog-fixture'

const roundTrip = (node: SpecNode, base = SALES_KEY) =>
    filterToNode(nodeToFilter(node, CATALOG, base), CATALOG, base)

describe('conditions', () => {
    it('round-trips each shape the builder writes', () => {
        const day = { op: 'addDays', args: [{ path: [{ field: 'day' }] }, { value: 0 }] }
        const shapes: SpecNode[] = [
            { op: 'contains', args: [{ path: [{ field: 'customer' }] }, { value: 'acme' }] },
            { op: 'gt', args: [{ path: [{ field: 'amount' }] }, { value: 50000 }] },
            { op: 'eq', args: [day, { value: '2026-01-05' }] },
            { op: 'lte', args: [day, { param: 'asOf' }] },
            { op: 'between', args: [day, { value: '2026-01-01' }, { value: '2026-01-31' }] },
            { op: 'between', args: [{ path: [{ field: 'day' }] }, { param: 'period' }] },
            {
                op: 'between',
                args: [{ path: [{ field: 'amount' }] }, { value: 100 }, { value: 200 }],
            },
            { op: 'in', args: [{ path: [{ field: 'status' }] }, { value: ['open', 'paid'] }] },
            { op: 'isEmpty', args: [{ path: [{ link: 'team' }, { field: 'name' }] }] },
            { op: 'not', args: [{ op: 'isEmpty', args: [{ path: [{ field: 'customer' }] }] }] },
        ]
        for (const node of shapes) expect(roundTrip(node)).toEqual(node)
    })

    it('reads a money value in cents and writes it back exactly', () => {
        const group = nodeToFilter(
            { op: 'gte', args: [{ path: [{ field: 'amount' }] }, { value: 123456 }] },
            CATALOG,
            SALES_KEY
        )
        const condition = group.items[0]
        expect(condition?.kind === 'condition' && condition.value).toBe('1234.56')
    })

    it('keeps a date comparison of another shape as a fixed part', () => {
        const raw: SpecNode = { op: 'lte', args: [{ path: [{ field: 'day' }] }, { param: 'asOf' }] }
        const group = nodeToFilter(raw, CATALOG, SALES_KEY)
        expect(group.items[0]?.kind).toBe('fixed')
        expect(filterToNode(group, CATALOG, SALES_KEY)).toEqual(raw)
    })
})

describe('groups and related rows', () => {
    it('round-trips all, any and none', () => {
        const a = { op: 'eq', args: [{ path: [{ field: 'status' }] }, { value: 'open' }] }
        const b = { op: 'contains', args: [{ path: [{ field: 'customer' }] }, { value: 'x' }] }
        expect(roundTrip({ op: 'and', args: [a, b] })).toEqual({ op: 'and', args: [a, b] })
        expect(roundTrip({ op: 'or', args: [a, b] })).toEqual({ op: 'or', args: [a, b] })
        expect(roundTrip({ op: 'not', args: [{ op: 'or', args: [a, b] }] })).toEqual({
            op: 'not',
            args: [{ op: 'or', args: [a, b] }],
        })
        expect(roundTrip({ op: 'and', args: [a, { op: 'or', args: [a, b] }] })).toEqual({
            op: 'and',
            args: [a, { op: 'or', args: [a, b] }],
        })
    })

    it('round-trips has and has no related rows with their own conditions', () => {
        const node: SpecNode = {
            op: 'exists',
            link: [{ link: 'sales' }],
            where: { op: 'gt', args: [{ path: [{ field: 'amount' }] }, { value: 50000 }] },
        }
        expect(roundTrip(node, TEAMS_KEY)).toEqual(node)
        expect(roundTrip({ op: 'notExists', link: [{ link: 'sales' }] }, TEAMS_KEY)).toEqual({
            op: 'notExists',
            link: [{ link: 'sales' }],
        })
    })

    it('writes no filter for an empty group, and keeps an unknown node as it is', () => {
        expect(
            filterToNode({ kind: 'group', match: 'all', items: [] }, CATALOG, SALES_KEY)
        ).toBeUndefined()
        const odd: SpecNode = {
            op: 'startsWith',
            args: [{ op: 'upper', args: [{ path: [{ field: 'customer' }] }] }, { value: 'A' }],
        }
        expect(roundTrip(odd)).toEqual(odd)
    })
})

describe('asking when run', () => {
    const sales = CATALOG.get(SALES_KEY)
    const field = (key: string) => {
        const found = sales?.fields.find(f => f.key === key)
        if (!found) throw new Error(key)
        return found
    }

    // R30: records asked when run can be listed in groups, so the reader picks a
    // whole group with "All <group>"; the spec keeps optionsFrom's own key order.
    it('groups the records a param lists, and stops grouping them', () => {
        const teams = CATALOG.get(TEAMS_KEY)
        const team: CatalogField = {
            key: 'id',
            label: 'Team',
            type: 'relation',
            aggregates: [],
        }
        const asked = askParamFor(team, 'in', 'team', teams)
        if (!asked?.optionsFrom) throw new Error('asked')
        const where: SpecNode = { value: true }
        const withWhere = { ...asked, optionsFrom: { ...asked.optionsFrom, where } }
        const grouped = withOptionsGroup(withWhere, [{ field: 'region' }])
        expect(grouped.optionsFrom).toEqual({
            source: TEAMS_KEY,
            value: [{ field: 'id' }],
            label: [{ field: 'name' }],
            group: [{ field: 'region' }],
            where,
        })
        expect(Object.keys(grouped.optionsFrom ?? {})).toEqual([
            'source',
            'value',
            'label',
            'group',
            'where',
        ])
        expect(withOptionsGroup(grouped, null)).toEqual(withWhere)
        // A param with its own options has no records to group.
        const own = askParamFor(field('status'), 'in', 's')
        if (!own) throw new Error('own')
        expect(withOptionsGroup(own, [{ field: 'region' }])).toBe(own)
    })

    it('makes a param that fits the field and operator', () => {
        expect(askParamFor(field('day'), 'between', 'period')).toEqual({
            id: 'period',
            type: 'dateRange',
            label: 'Day',
        })
        expect(askParamFor(field('day'), 'lte', 'asOf')).toEqual({
            id: 'asOf',
            type: 'asOf',
            label: 'Day',
        })
        expect(askParamFor(field('customer'), 'contains', 'who')).toEqual({
            id: 'who',
            type: 'text',
            label: 'Customer',
        })
        expect(askParamFor(field('status'), 'in', 's')).toEqual({
            id: 's',
            type: 'choice',
            label: 'Status',
            multiple: true,
            options: [
                { value: 'open', label: 'Open' },
                { value: 'paid', label: 'Paid' },
            ],
        })
        expect(askParamFor(field('amount'), 'gt', 'a')).toEqual({
            id: 'a',
            type: 'amount',
            label: 'Amount',
        })
        expect(askParamFor(field('day'), 'in', 'd')).toBeNull()
        expect(askParamFor(field('customer'), 'isEmpty', 'e')).toBeNull()
    })

    it('lists the params a filter asks for', () => {
        const group = nodeToFilter(
            {
                op: 'and',
                args: [
                    { op: 'between', args: [{ path: [{ field: 'day' }] }, { param: 'period' }] },
                    {
                        op: 'startsWith',
                        args: [{ op: 'upper', args: [{ param: 'raw' }] }, { value: 'A' }],
                    },
                ],
            },
            CATALOG,
            SALES_KEY
        )
        expect([...paramsInGroup(group, new Set())].sort()).toEqual(['period', 'raw'])
    })
})

describe('client checks', () => {
    it('names a condition with a missing or wrong value', () => {
        const group: GroupDraft = {
            kind: 'group',
            match: 'all',
            items: [
                { ...emptyCondition([{ field: 'amount' }]), op: 'gt', value: 'lots' },
                { ...emptyCondition([{ field: 'day' }]), op: 'eq', value: '2026-02-30' },
                { ...emptyCondition([{ field: 'customer' }]), op: 'contains', value: '' },
                { ...emptyCondition([{ field: 'customer' }]), op: 'isEmpty' },
            ],
        }
        expect(conditionProblems(group, CATALOG, SALES_KEY)).toEqual([
            'Amount: enter an amount such as 12.50',
            'Day: enter a date as YYYY-MM-DD',
            'Customer: enter a value',
        ])
    })

    it('offers operators by field type', () => {
        expect(opsFor('bool')).toEqual(['isTrue', 'isFalse'])
        expect(opsFor('select')).toEqual(['eq', 'ne', 'in', 'isEmpty', 'notEmpty'])
        expect(opsFor('date')).toContain('between')
    })

    it('keeps a read-back operator the field type does not offer in the picker', () => {
        expect(opsShownFor('date', 'gte')).toEqual(opsFor('date'))
        expect(opsShownFor('date', 'ne')).toEqual(['ne', ...opsFor('date')])
        expect(opsShownFor('text', 'lt')[0]).toBe('lt')
        expect(opsShownFor('bool', 'eq')).toEqual(['eq', 'isTrue', 'isFalse'])
    })
})

describe('relation conditions', () => {
    const teamLink = {
        key: 'id',
        label: 'Team',
        type: 'relation' as const,
        aggregates: ['count' as const, 'countDistinct' as const],
    }
    const teams = CATALOG.get(TEAMS_KEY)

    it('offers is, is not, is any of, is empty and is not empty', () => {
        expect(opsFor('relation')).toEqual(['eq', 'ne', 'in', 'isEmpty', 'notEmpty'])
    })

    it('asks a record picked from the target’s records', () => {
        expect(askParamFor(teamLink, 'in', 'team', teams)).toEqual({
            id: 'team',
            type: 'choice',
            label: 'Team',
            multiple: true,
            optionsFrom: {
                source: TEAMS_KEY,
                value: [{ field: 'id' }],
                label: [{ field: 'name' }],
            },
        })
        expect(askParamFor(teamLink, 'eq', 'team', teams)).toEqual({
            id: 'team',
            type: 'choice',
            label: 'Team',
            optionsFrom: {
                source: TEAMS_KEY,
                value: [{ field: 'id' }],
                label: [{ field: 'name' }],
            },
        })
        expect(askParamFor(teamLink, 'in', 'team')).toBeNull()
    })

    it('round-trips each relation shape', () => {
        const team = { path: [{ link: 'team' }] }
        const shapes: SpecNode[] = [
            { op: 'eq', args: [team, { value: 'abc123' }] },
            { op: 'ne', args: [team, { param: 'team' }] },
            { op: 'in', args: [team, { value: ['a', 'b'] }] },
            { op: 'in', args: [team, { param: 'teams' }] },
            { op: 'isEmpty', args: [team] },
            { op: 'not', args: [{ op: 'isEmpty', args: [team] }] },
        ]
        for (const node of shapes) {
            expect(nodeToFilter(node, CATALOG, SALES_KEY).items[0]?.kind).toBe('condition')
            expect(roundTrip(node)).toEqual(node)
        }
    })
})

describe('conditions on a group level', () => {
    const sold = { key: 'quantity', label: 'Qty sold', type: 'number' as const, aggregates: [] }
    const level: LevelScope = {
        columns: [{ key: 'quantity', label: 'Qty sold', field: sold }],
        records: [{ group: 'byTeam', label: 'Team', source: TEAMS_KEY }],
    }
    const levelTrip = (node: SpecNode) =>
        filterToNode(nodeToFilter(node, CATALOG, SALES_KEY, level), CATALOG, SALES_KEY, level)

    it('writes a total condition and reads it back to the same draft', () => {
        const condition: ConditionDraft = {
            ...emptyCondition([{ field: 'amount' }]),
            subject: 'total',
            aggregate: 'sum',
            op: 'gt',
            value: '10.00',
        }
        const group: GroupDraft = { kind: 'group', match: 'all', items: [condition] }
        const node = filterToNode(group, CATALOG, SALES_KEY, level)
        expect(node).toEqual({
            op: 'gt',
            args: [{ op: 'sum', args: [{ path: [{ field: 'amount' }] }] }, { value: 1000 }],
        })
        expect(nodeToFilter(node, CATALOG, SALES_KEY, level)).toEqual(group)
    })

    it('round-trips a column, a count and a group record condition', () => {
        const shapes: SpecNode[] = [
            { op: 'gte', args: [{ col: 'quantity' }, { value: 3 }] },
            { op: 'lt', args: [{ col: 'quantity' }, { param: 'least' }] },
            {
                op: 'gt',
                args: [{ op: 'count', args: [{ path: [{ field: 'id' }] }] }, { value: 2 }],
            },
            {
                op: 'lte',
                args: [
                    { op: 'max', args: [{ path: [{ field: 'day' }] }] },
                    { value: '2026-01-31' },
                ],
            },
            {
                op: 'eq',
                args: [{ record: 'byTeam', path: [{ field: 'region' }] }, { value: 'north' }],
            },
            { op: 'isEmpty', args: [{ record: 'byTeam', path: [{ field: 'name' }] }] },
        ]
        for (const node of shapes) {
            expect(nodeToFilter(node, CATALOG, SALES_KEY, level).items[0]?.kind).toBe('condition')
            expect(levelTrip(node)).toEqual(node)
        }
        expect(nodeToFilter(shapes[2], CATALOG, SALES_KEY, level).items[0]).toMatchObject({
            subject: 'total',
            aggregate: 'count',
        })
        expect(nodeToFilter(shapes[4], CATALOG, SALES_KEY, level).items[0]).toMatchObject({
            subject: 'record',
            record: 'byTeam',
            path: [{ field: 'region' }],
        })
    })

    it('reads a column, a total or a record as fixed outside a group level', () => {
        const node: SpecNode = { op: 'gte', args: [{ col: 'quantity' }, { value: 3 }] }
        expect(nodeToFilter(node, CATALOG, SALES_KEY).items[0]?.kind).toBe('fixed')
        const unknown: SpecNode = { op: 'gte', args: [{ col: 'gone' }, { value: 3 }] }
        expect(nodeToFilter(unknown, CATALOG, SALES_KEY, level).items[0]?.kind).toBe('fixed')
    })

    // A problem names a column by its label and a total by its aggregate, never an id.
    it('names a level condition with a missing column or value', () => {
        const group: GroupDraft = {
            kind: 'group',
            match: 'all',
            items: [
                { ...emptyCondition([]), subject: 'column', col: 'gone', op: 'gt', value: '1' },
                { ...emptyCondition([]), subject: 'column', col: 'quantity', op: 'gt', value: 'x' },
                {
                    ...emptyCondition([{ field: 'amount' }]),
                    subject: 'total',
                    aggregate: 'sum',
                    op: 'gt',
                    value: '',
                },
                {
                    ...emptyCondition([{ field: 'region' }]),
                    subject: 'record',
                    record: 'byTeam',
                    op: 'eq',
                    value: '',
                },
            ],
        }
        expect(conditionProblems(group, CATALOG, SALES_KEY, level)).toEqual([
            'A condition names a column that is no longer in the report',
            'Qty sold: enter a number',
            'Sum of Amount: enter a value',
            'Team › Region: enter a value',
        ])
    })
})

describe('asked amounts and ranges', () => {
    const amount = {
        key: 'amount',
        label: 'Amount',
        type: 'money' as const,
        aggregates: ['sum' as const],
    }
    const quantity = { key: 'quantity', label: 'Quantity', type: 'number' as const, aggregates: [] }
    it('asks a money value as an amount param', () => {
        expect(askParamFor(amount, 'gte', 'amount')).toEqual({
            id: 'amount',
            type: 'amount',
            label: 'Amount',
        })
    })
    it('asks a range of numbers or amounts as two params', () => {
        expect(askParamFor(quantity, 'between', 'quantity')).toEqual({
            id: 'quantity',
            type: 'number',
            label: 'Quantity from',
        })
        expect(askHighParamFor(quantity, 'between', 'quantity')).toEqual({
            id: 'quantityTo',
            type: 'number',
            label: 'Quantity to',
        })
        expect(askHighParamFor(amount, 'between', 'amount')).toEqual({
            id: 'amountTo',
            type: 'amount',
            label: 'Amount to',
        })
        expect(askHighParamFor(quantity, 'gte', 'quantity')).toBeNull()
    })
    it('writes and reads a two-param between', () => {
        const node = {
            op: 'between',
            args: [
                { path: [{ field: 'quantity' }] },
                { param: 'quantity' },
                { param: 'quantityTo' },
            ],
        }
        const group = nodeToFilter(node, CATALOG, SALES_KEY)
        expect(group.items[0]).toMatchObject({
            kind: 'condition',
            op: 'between',
            param: 'quantity',
            paramHigh: 'quantityTo',
        })
        expect(filterToNode(group, CATALOG, SALES_KEY)).toEqual(node)
        expect([...paramsInGroup(group, new Set())]).toEqual(['quantity', 'quantityTo'])
    })
})

describe('a fill condition that matches the group above', () => {
    const above = [{ id: 'g0', col: 'region', label: 'Region' }]
    it('reads {col} of an enclosing group as "same as the group above" and writes it back', () => {
        const node = { op: 'eq', args: [{ path: [{ field: 'region' }] }, { col: 'region' }] }
        const group = nodeToFilter(node, CATALOG, TEAMS_KEY, undefined, above)
        expect(group.items[0]).toMatchObject({
            kind: 'condition',
            op: 'eq',
            above: 'g0',
            param: '',
            value: '',
        })
        expect(filterToNode(group, CATALOG, TEAMS_KEY, undefined, above)).toEqual(node)
        expect(conditionProblems(group, CATALOG, TEAMS_KEY, undefined, above)).toEqual([])
    })
    it('reads a {col} that is no enclosing group as a fixed part', () => {
        const node = { op: 'eq', args: [{ path: [{ field: 'region' }] }, { col: 'other' }] }
        expect(nodeToFilter(node, CATALOG, TEAMS_KEY, undefined, above).items[0]).toMatchObject({
            kind: 'fixed',
        })
    })
    it('names a condition whose group above is gone', () => {
        const node = { op: 'eq', args: [{ path: [{ field: 'region' }] }, { col: 'region' }] }
        const group = nodeToFilter(node, CATALOG, TEAMS_KEY, undefined, above)
        expect(conditionProblems(group, CATALOG, TEAMS_KEY, undefined, [])).toEqual([
            'Region: the group above is no longer in the report',
        ])
    })
    it('lists the groups above a level, named by their columns', () => {
        const groups = [
            { id: 'g0', col: 'region' },
            { id: 'g1', col: 'team' },
            { id: 'g2', col: 'gone' },
        ]
        const columns = [
            { id: 'region', label: 'Region' },
            { id: 'team', label: 'Team' },
        ]
        expect(aboveOf(groups, 0, columns)).toEqual([])
        expect(aboveOf(groups, 2, columns)).toEqual([
            { id: 'g0', col: 'region', label: 'Region' },
            { id: 'g1', col: 'team', label: 'Team' },
        ])
        expect(aboveOf(groups, 3, columns)[2]).toEqual({ id: 'g2', col: 'gone', label: 'gone' })
    })
})

describe('an average on a group level', () => {
    const level: LevelScope = { columns: [], records: [] }
    const avg = (field: string, scale?: number): SpecNode => ({
        op: 'avg',
        args: [{ path: [{ field }] }],
        ...(scale === undefined ? {} : { scale }),
    })
    const gt = (subject: SpecNode, value: number): SpecNode => ({
        op: 'gt',
        args: [subject, { value }],
    })

    it('writes the scale the engine needs: whole cents for amounts, else the number default', () => {
        const condition = (field: string): GroupDraft => ({
            kind: 'group',
            match: 'all',
            items: [
                {
                    ...emptyCondition([{ field }]),
                    subject: 'total',
                    aggregate: 'avg',
                    op: 'gt',
                    value: '5',
                },
            ],
        })
        expect(filterToNode(condition('amount'), CATALOG, SALES_KEY, level)).toEqual(
            gt(avg('amount', 0), 500)
        )
        expect(filterToNode(condition('quantity'), CATALOG, SALES_KEY, level)).toEqual(
            gt(avg('quantity', 2), 5)
        )
    })

    it('reads an average with its scale as an editable total and keeps the scale', () => {
        for (const node of [
            gt(avg('amount', 0), 500),
            gt(avg('quantity', 2), 5),
            gt(avg('quantity', 4), 5),
        ]) {
            const group = nodeToFilter(node, CATALOG, SALES_KEY, level)
            expect(group.items[0]).toMatchObject({
                kind: 'condition',
                subject: 'total',
                aggregate: 'avg',
            })
            expect(filterToNode(group, CATALOG, SALES_KEY, level)).toEqual(node)
        }
        expect(
            nodeToFilter(gt(avg('quantity', 4), 5), CATALOG, SALES_KEY, level).items[0]
        ).toMatchObject({
            scale: 4,
        })
    })

    it('keeps an average the engine refuses as a fixed part', () => {
        for (const node of [gt(avg('quantity'), 5), gt(avg('amount', 2), 500)]) {
            expect(nodeToFilter(node, CATALOG, SALES_KEY, level).items[0]?.kind).toBe('fixed')
        }
    })
})

describe('a value that is the key of the group above', () => {
    const above = [{ id: 'g0', col: 'region', label: 'Region' }]
    it('asks nothing, even when the condition still names a param', () => {
        const group: GroupDraft = {
            kind: 'group',
            match: 'all',
            items: [
                {
                    ...emptyCondition([{ field: 'region' }]),
                    op: 'eq',
                    above: 'g0',
                    param: 'stale',
                    paramHigh: 'staleTo',
                },
            ],
        }
        expect([...paramsInGroup(group, new Set())]).toEqual([])
        expect(filterToNode(group, CATALOG, TEAMS_KEY, undefined, above)).toEqual({
            op: 'eq',
            args: [{ path: [{ field: 'region' }] }, { col: 'region' }],
        })
        expect(conditionProblems(group, CATALOG, TEAMS_KEY, undefined, above)).toEqual([])
    })
})
