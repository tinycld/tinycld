import { describe, expect, it } from 'vitest'
import type { SpecNode } from '../../spec'
import {
    addOperandNode,
    canRemoveNode,
    contextAt,
    describeNode,
    formulaType,
    isEditableNode,
    partOf,
    partsAt,
    type ReportFormulaContext,
    removeOperandNode,
    slotLabel,
    withPart,
} from '../formula-ops'
import { CATALOG, SALES_KEY, TEAMS_KEY } from './catalog-fixture'

const CTX: ReportFormulaContext = {
    catalog: CATALOG,
    leaf: 'col',
    values: [
        { key: 'amount', label: 'Amount', type: 'money' },
        { key: 'cost', label: 'Cost', type: 'money' },
        { key: 'qty', label: 'Quantity', type: 'number' },
    ],
    source: SALES_KEY,
    records: [{ group: 'byTeam', label: 'Team', source: TEAMS_KEY }],
    params: [{ id: 'asOf', label: 'As of', type: 'asOf' }],
    totals: true,
    windows: true,
    lookups: true,
}
const LAYOUT: ReportFormulaContext = {
    ...CTX,
    leaf: 'subtotal',
    source: null,
    records: [],
    params: [],
    totals: false,
    windows: false,
    lookups: false,
}
const WITH_PERIOD: ReportFormulaContext = {
    ...CTX,
    params: [
        { id: 'period', label: 'Period', type: 'dateRange' },
        { id: 'asOf', label: 'As of', type: 'asOf' },
    ],
}

const LARGER: SpecNode = {
    op: 'if',
    args: [
        { op: 'gt', args: [{ col: 'amount' }, { col: 'cost' }] },
        { col: 'amount' },
        { col: 'cost' },
    ],
}
const DEBIT: SpecNode = {
    op: 'if',
    args: [
        { op: 'eq', args: [{ op: 'count', args: [{ path: [{ field: 'id' }] }] }, { value: 0 }] },
        { value: null },
        {
            op: 'if',
            args: [
                {
                    op: 'gte',
                    args: [{ op: 'sum', args: [{ path: [{ field: 'amount' }] }] }, { value: 0 }],
                },
                { op: 'sum', args: [{ path: [{ field: 'amount' }] }] },
                { value: null },
            ],
        },
    ],
}
const LOOKUP: SpecNode = {
    op: 'lookup',
    source: TEAMS_KEY,
    where: { op: 'eq', args: [{ path: [{ field: 'name' }] }, { value: 'North' }] },
    value: { path: [{ field: 'id' }] },
    pick: 'min',
}
const IN_PERIOD: SpecNode = {
    op: 'between',
    args: [{ path: [{ field: 'day' }] }, { param: 'period' }],
}

describe('which formulas the editor shows', () => {
    it('edits every report operator', () => {
        expect(isEditableNode(LARGER)).toBe(true)
        expect(isEditableNode(DEBIT)).toBe(true)
        expect(isEditableNode(LOOKUP)).toBe(true)
        expect(isEditableNode({ record: 'byTeam', path: [{ field: 'name' }] })).toBe(true)
        expect(isEditableNode({ op: 'concat', args: [{ col: 'amount' }, { value: ' · ' }] })).toBe(
            true
        )
    })
    it('keeps read-only what it cannot write', () => {
        expect(isEditableNode({ op: 'exists', link: [{ link: 'team' }] })).toBe(false)
        expect(isEditableNode({ op: 'div', args: [{ col: 'amount' }, { value: 2 }] })).toBe(false)
        expect(isEditableNode({ op: 'shout', args: [] })).toBe(false)
        expect(isEditableNode({ op: 'not', args: [{ value: true }, { value: false }] })).toBe(false)
        // A part id the engine has no operator for is not an operator.
        expect(isEditableNode({ op: 'smallest', args: [{ col: 'amount' }] })).toBe(false)
        expect(isEditableNode({ op: 'inPeriod', args: [{ col: 'amount' }, { param: 'p' }] })).toBe(
            false
        )
        expect(isEditableNode({ op: 'runningSum', args: [{ col: 'qty' }], scale: 2 })).toBe(false)
    })
    it('edits the engine forms beyond the plain operators: a period test and a list of numbers', () => {
        expect(isEditableNode(IN_PERIOD)).toBe(true)
        expect(isEditableNode({ op: 'in', args: [{ col: 'qty' }, { value: [1, 2] }] })).toBe(true)
        expect(isEditableNode({ op: 'between', args: [{ col: 'qty' }, { value: 2 }] })).toBe(false)
        expect(isEditableNode({ value: ['a', 1] })).toBe(false)
    })
    it('keeps read-only a path hop key it does not write', () => {
        expect(
            isEditableNode({ path: [{ link: 'team', with: 'required' }, { field: 'name' }] })
        ).toBe(false)
        expect(isEditableNode({ path: [{ link: 'team', field: 'name' }] })).toBe(false)
        expect(isEditableNode({ path: [{ link: 'team' }] })).toBe(true)
    })
})

describe('changing a part', () => {
    it('keeps the forms part ids for the shared parts', () => {
        expect(partOf({ value: 0 })).toBe('number')
        expect(partOf({ col: 'amount' })).toBe('field')
        expect(partOf({ op: 'min', args: [{ col: 'amount' }] })).toBe('smallest')
        expect(partOf({ op: 'min', args: [{ col: 'amount' }, { col: 'cost' }] })).toBe('min')
        expect(partOf(IN_PERIOD)).toBe('inPeriod')
    })
    it('carries operands over, wraps a part, and divides amounts by numbers in whole cents', () => {
        const sub = withPart(CTX, { value: 0 }, [], 'sub')
        expect(sub).toEqual({ op: 'sub', args: [{ value: 0 }, { value: 0 }] })
        expect(withPart(CTX, { col: 'amount' }, [], 'if')).toEqual({
            op: 'if',
            args: [{ value: true }, { col: 'amount' }, { value: null }],
        })
        expect(
            withPart(CTX, { op: 'add', args: [{ col: 'amount' }, { col: 'qty' }] }, [], 'div')
        ).toEqual({
            op: 'div',
            args: [{ col: 'amount' }, { col: 'qty' }],
            scale: 0,
        })
        expect(
            withPart(CTX, { op: 'add', args: [{ col: 'qty' }, { col: 'qty' }] }, [], 'div')
        ).toMatchObject({
            scale: 4,
        })
        expect(withPart(CTX, { value: 0 }, [], 'field')).toEqual({ col: 'amount' })
        expect(withPart(LAYOUT, { value: 0 }, [], 'field')).toEqual({ subtotal: 'amount' })
        expect(withPart(CTX, { value: 0 }, [], 'record')).toEqual({
            record: 'byTeam',
            path: [{ field: 'name' }],
        })
        expect(withPart(CTX, { value: 0 }, [], 'lookup')).toMatchObject({
            op: 'lookup',
            pick: 'min',
            where: { value: true },
        })
    })
    it('averages amounts in whole cents, as the engine requires', () => {
        expect(withPart(CTX, { col: 'amount' }, [], 'avg')).toEqual({
            op: 'avg',
            args: [{ col: 'amount' }],
            scale: 0,
        })
        expect(withPart(CTX, { col: 'qty' }, [], 'avg')).toMatchObject({ scale: 2 })
    })
    it('tests a date against a date range param, and asks a value that is not a range', () => {
        expect(withPart(WITH_PERIOD, { path: [{ field: 'day' }] }, [], 'inPeriod')).toEqual(
            IN_PERIOD
        )
        expect(withPart(CTX, { path: [{ field: 'day' }] }, [], 'inPeriod')).toEqual({
            path: [{ field: 'day' }],
        })
        expect(withPart(WITH_PERIOD, { value: 0 }, [], 'param')).toEqual({ param: 'asOf' })
        expect(partsAt(WITH_PERIOD, { value: 0 }, [])).toContain('inPeriod')
        expect(partsAt(CTX, { value: 0 }, [])).not.toContain('inPeriod')
        expect([0, 1].map(i => slotLabel(IN_PERIOD, i))).toEqual(['Value', 'Period'])
    })
    it('adds and removes a case pair before its otherwise', () => {
        const c: SpecNode = { op: 'case', args: [{ value: true }, { value: 1 }, { value: 2 }] }
        const added = addOperandNode(c)
        expect(added.args).toEqual([
            { value: true },
            { value: 1 },
            { value: true },
            { value: null },
            { value: 2 },
        ])
        expect(removeOperandNode(added, 2).args).toEqual(c.args)
        expect([0, 1, 2].map(i => slotLabel(c, i))).toEqual(['When', 'Then', 'Otherwise'])
        expect([0, 1].map(i => slotLabel(LOOKUP, i))).toEqual(['Where', 'Value'])
    })
    it('adds and removes operands within the operator arity', () => {
        const and: SpecNode = { op: 'and', args: [{ value: true }, { value: false }] }
        expect(canRemoveNode(and)).toBe(false)
        const three = addOperandNode(and)
        expect(three.args).toEqual([{ value: true }, { value: false }, { value: true }])
        expect(removeOperandNode(three, 0).args).toEqual([{ value: false }, { value: true }])
        expect(addOperandNode({ op: 'sub', args: [{ value: 1 }, { value: 2 }] }).args).toHaveLength(
            2
        )
    })
})

describe('what a part offers where it sits', () => {
    it('offers no path, record, param, total or lookup in a layout row', () => {
        const parts = partsAt(LAYOUT, { value: 0 }, [])
        for (const p of ['path', 'record', 'param', 'sum', 'lookup', 'runningSum'])
            expect(parts).not.toContain(p)
        expect(parts).toContain('field')
    })
    it('offers every report operator but exists in a column', () => {
        const parts = partsAt(CTX, { value: 0 }, [])
        for (const p of [
            'lookup',
            'sum',
            'runningSum',
            'percentOfTotal',
            'case',
            'coalesce',
            'fiscalPeriod',
            'diffDays',
        ])
            expect(parts).toContain(p)
        expect(parts).not.toContain('exists')
    })
    it('reads the looked-up source inside a lookup, with no totals', () => {
        const inside = contextAt(CTX, LOOKUP, [1])
        expect(inside.source).toBe(TEAMS_KEY)
        expect(inside.totals).toBe(false)
        expect(contextAt(CTX, { op: 'sum', args: [{ col: 'qty' }] }, [0]).totals).toBe(false)
        expect(contextAt(CTX, { op: 'runningSum', args: [{ col: 'qty' }] }, [0]).windows).toBe(
            false
        )
    })
})

describe('reading a formula back', () => {
    it('says it as a person would', () => {
        expect(describeNode(CTX, { op: 'sub', args: [{ col: 'amount' }, { col: 'cost' }] })).toBe(
            'Amount − Cost'
        )
        expect(describeNode(CTX, LARGER)).toBe('if Amount > Cost then Amount else Cost')
        expect(describeNode(CTX, LOOKUP)).toBe(
            'look up ID in Stub teams where Name = "North" (smallest)'
        )
        expect(describeNode(CTX, { record: 'byTeam', path: [{ field: 'name' }] })).toBe(
            'Team › Name'
        )
        expect(describeNode(WITH_PERIOD, IN_PERIOD)).toBe('Day is in {Period}')
    })
    it('types it as the engine does', () => {
        const types = {
            catalog: CATALOG,
            source: SALES_KEY,
            col: (k: string) => CTX.values.find(v => v.key === k)?.type,
            records: CTX.records,
            params: CTX.params,
        }
        expect(formulaType(LARGER, types)).toBe('money')
        expect(
            formulaType({ op: 'div', args: [{ col: 'amount' }, { col: 'qty' }], scale: 0 }, types)
        ).toBe('money')
        expect(formulaType({ op: 'gt', args: [{ col: 'qty' }, { value: 1 }] }, types)).toBe('bool')
        expect(formulaType({ op: 'count', args: [{ path: [{ field: 'customer' }] }] }, types)).toBe(
            'number'
        )
        expect(formulaType({ op: 'concat', args: [{ value: 'a' }] }, types)).toBe('text')
        expect(formulaType(LOOKUP, types)).toBe('relation')
        expect(formulaType({ path: [{ link: 'team' }] }, types)).toBe('relation')
        const pct = { op: 'percentOfTotal', args: [{ col: 'qty' }], scale: 2 }
        expect(formulaType({ op: 'add', args: [pct, { value: 1 }] }, types)).toBe('percent')
        expect(formulaType({ op: 'avg', args: [pct], scale: 2 }, types)).toBe('number')
        expect(
            formulaType(
                { op: 'coalesce', args: [{ value: null }, { path: [{ field: 'day' }] }] },
                types
            )
        ).toBe('date')
    })
})
