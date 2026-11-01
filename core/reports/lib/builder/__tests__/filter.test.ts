import { describe, expect, it } from 'vitest'
import type { SpecNode } from '../../spec'
import {
    askParamFor,
    centsFromText,
    conditionProblems,
    emptyCondition,
    filterToNode,
    type GroupDraft,
    nodeToFilter,
    opsFor,
    opsShownFor,
    paramsInGroup,
    textFromCents,
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
        expect(centsFromText('1234.5')).toBe(123450)
        expect(centsFromText('-0.07')).toBe(-7)
        expect(centsFromText('12.345')).toBeNull()
        expect(textFromCents(-7)).toBe('-0.07')
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
        expect(askParamFor(field('amount'), 'gt', 'a')).toBeNull()
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
