import { replaceTreeAt } from '@tinycld/core/lib/forms/builder/formula-tree'
import { describe, expect, it } from 'vitest'
import type { SpecNode } from '../../spec'
import {
    addOperandNode,
    addOtherwise,
    canRemoveNode,
    contextAt,
    forcedScaleAt,
    hasOtherwise,
    isEditableNode,
    isNumberList,
    paramsAt,
    partsAt,
    removeOperandNode,
    removeOtherwise,
    settleScales,
    slotLabel,
    withPart,
} from '../formula-ops'
import { boundChoices, LOOKUP_PICKS, partOf, REPORT_TREE } from '../formula-parts'
import { SALES_KEY, TEAMS_KEY } from './catalog-fixture'
import {
    CTX,
    DEBIT,
    IN_PERIOD,
    LARGER,
    LAYOUT,
    LOOKUP,
    TEAM_WITH_SALES,
    WITH_PERIOD,
} from './formula-fixture'

const PARAMS = WITH_PERIOD.params

describe('which formulas the editor shows', () => {
    it('edits every report operator', () => {
        expect(isEditableNode(LARGER, PARAMS)).toBe(true)
        expect(isEditableNode(DEBIT, PARAMS)).toBe(true)
        expect(isEditableNode(LOOKUP, PARAMS)).toBe(true)
        expect(isEditableNode({ record: 'byTeam', path: [{ field: 'name' }] }, PARAMS)).toBe(true)
        expect(
            isEditableNode({ op: 'concat', args: [{ col: 'amount' }, { value: ' · ' }] }, PARAMS)
        ).toBe(true)
    })
    it('keeps read-only what it cannot write', () => {
        expect(isEditableNode({ op: 'exists', link: [{ link: 'team' }] }, PARAMS)).toBe(false)
        expect(isEditableNode({ op: 'div', args: [{ col: 'amount' }, { value: 2 }] }, PARAMS)).toBe(
            false
        )
        expect(isEditableNode({ op: 'shout', args: [] }, PARAMS)).toBe(false)
        expect(
            isEditableNode({ op: 'not', args: [{ value: true }, { value: false }] }, PARAMS)
        ).toBe(false)
        // A part id the engine has no operator for is not an operator.
        expect(isEditableNode({ op: 'smallest', args: [{ col: 'amount' }] }, PARAMS)).toBe(false)
        expect(
            isEditableNode(
                { op: 'inPeriod', args: [{ col: 'amount' }, { param: 'period' }] },
                PARAMS
            )
        ).toBe(false)
        expect(isEditableNode({ op: 'runningSum', args: [{ col: 'qty' }], scale: 2 }, PARAMS)).toBe(
            false
        )
    })
    it('edits a period test over a date range param, and a list of numbers', () => {
        expect(isEditableNode(IN_PERIOD, PARAMS)).toBe(true)
        expect(
            isEditableNode({ op: 'in', args: [{ col: 'qty' }, { value: [1, 2] }] }, PARAMS)
        ).toBe(true)
        expect(
            isEditableNode({ op: 'between', args: [{ col: 'qty' }, { value: 2 }] }, PARAMS)
        ).toBe(false)
        expect(isEditableNode({ value: ['a', 1] }, PARAMS)).toBe(false)
    })
    it('refuses a period test whose period is not a date range', () => {
        const asOf: SpecNode = {
            op: 'between',
            args: [{ path: [{ field: 'day' }] }, { param: 'asOf' }],
        }
        expect(isEditableNode(asOf, PARAMS)).toBe(false)
        expect(isEditableNode(IN_PERIOD, CTX.params)).toBe(false)
    })
    // A link's `with` keeps or drops rows without the link (bind.go hopPlan.required);
    // `mode` belongs to a column's to-many link, which a value path refuses.
    it('edits a link that keeps only rows with it, and keeps read-only a hop key it does not write', () => {
        const required = { path: [{ link: 'team', with: 'required' as const }, { field: 'name' }] }
        expect(isEditableNode(required, PARAMS)).toBe(true)
        const record = { record: 'byTeam', path: [{ link: 'team', with: 'optional' as const }] }
        expect(isEditableNode(record, PARAMS)).toBe(true)
        const mode = { path: [{ link: 'sales', mode: 'aggregate' as const }, { field: 'amount' }] }
        expect(isEditableNode(mode, PARAMS)).toBe(false)
        expect(isEditableNode({ path: [{ link: 'team', field: 'name' }] }, PARAMS)).toBe(false)
        expect(isEditableNode({ path: [{ link: 'team' }] }, PARAMS)).toBe(true)
    })
    it('edits an exists only inside a lookup', () => {
        expect(isEditableNode(TEAM_WITH_SALES, PARAMS)).toBe(true)
        const where = TEAM_WITH_SALES.where
        expect(where && isEditableNode(where, PARAMS)).toBe(false)
        const withRequired: SpecNode = {
            ...TEAM_WITH_SALES,
            where: { op: 'exists', link: [{ link: 'sales', with: 'required' }] },
        }
        expect(isEditableNode(withRequired, PARAMS)).toBe(false)
    })
})

describe('the tree', () => {
    const shapes: SpecNode[] = [
        LARGER,
        DEBIT,
        LOOKUP,
        IN_PERIOD,
        TEAM_WITH_SALES,
        { op: 'lookup', source: TEAMS_KEY, value: { path: [{ field: 'name' }] } },
        { op: 'notExists', link: [{ link: 'sales' }] },
        { op: 'case', args: [{ value: true }, { value: 1 }, { value: 2 }] },
        { op: 'div', args: [{ col: 'amount' }, { col: 'qty' }], scale: 0 },
    ]
    const nodesOf = (node: SpecNode): SpecNode[] => [
        node,
        ...REPORT_TREE.childrenOf(node).flatMap(nodesOf),
    ]

    it('gives every editable node back as it was when its operands are put back', () => {
        for (const node of shapes.flatMap(nodesOf)) {
            expect(REPORT_TREE.withChildren(node, REPORT_TREE.childrenOf(node))).toStrictEqual(node)
        }
    })
    it('keeps a lookup or an exists without a where while its slot still says yes', () => {
        const noWhere: SpecNode = {
            op: 'lookup',
            source: TEAMS_KEY,
            value: { path: [{ field: 'name' }] },
        }
        const edited = replaceTreeAt(REPORT_TREE, noWhere, [1], { path: [{ field: 'id' }] })
        expect(edited).toStrictEqual({ ...noWhere, value: { path: [{ field: 'id' }] } })
        const condition: SpecNode = { op: 'eq', args: [{ value: 1 }, { value: 1 }] }
        expect(replaceTreeAt(REPORT_TREE, noWhere, [0], condition).where).toEqual(condition)
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
        expect(withPart(CTX, { value: 0 }, [], 'sub')).toEqual({
            op: 'sub',
            args: [{ value: 0 }, { value: 0 }],
        })
        expect(withPart(CTX, { col: 'amount' }, [], 'if')).toEqual({
            op: 'if',
            args: [{ value: true }, { col: 'amount' }, { value: null }],
        })
        const sum: SpecNode = { op: 'add', args: [{ col: 'amount' }, { col: 'qty' }] }
        expect(withPart(CTX, sum, [], 'div')).toEqual({
            op: 'div',
            args: [{ col: 'amount' }, { col: 'qty' }],
            scale: 0,
        })
        const qtys: SpecNode = { op: 'add', args: [{ col: 'qty' }, { col: 'qty' }] }
        expect(withPart(CTX, qtys, [], 'div')).toMatchObject({ scale: 4 })
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
    it('tests a date against a date range param, and asks a value that fits a single value', () => {
        const day: SpecNode = { path: [{ field: 'day' }] }
        expect(withPart(WITH_PERIOD, day, [], 'inPeriod')).toEqual(IN_PERIOD)
        expect(withPart(CTX, day, [], 'inPeriod')).toEqual(day)
        expect(withPart(WITH_PERIOD, { value: 0 }, [], 'param')).toEqual({ param: 'asOf' })
        expect([0, 1].map(i => slotLabel(IN_PERIOD, i))).toEqual(['Value', 'Period'])
    })
    it('never carries a date range into another part', () => {
        expect(withPart(WITH_PERIOD, IN_PERIOD, [], 'add')).toEqual({
            op: 'add',
            args: [{ path: [{ field: 'day' }] }, { value: 0 }],
        })
        const period: SpecNode = { param: 'period' }
        expect(withPart(WITH_PERIOD, period, [], 'if')).toEqual({
            op: 'if',
            args: [{ value: true }, { value: null }, { value: null }],
        })
        expect(withPart(WITH_PERIOD, period, [], 'year')).toEqual({
            op: 'year',
            args: [{ value: 0 }],
        })
        expect(withPart(WITH_PERIOD, { param: 'statuses' }, [], 'upper')).toEqual({
            op: 'upper',
            args: [{ value: 0 }],
        })
    })
    it('moves a tested value between Is between, Is in period and Is any of', () => {
        const qty: SpecNode = { col: 'qty' }
        const between: SpecNode = { op: 'between', args: [qty, { value: 1 }, { value: 9 }] }
        expect(withPart(WITH_PERIOD, between, [], 'inPeriod')).toEqual({
            op: 'between',
            args: [qty, { param: 'period' }],
        })
        expect(withPart(WITH_PERIOD, IN_PERIOD, [], 'in')).toEqual({
            op: 'in',
            args: [{ path: [{ field: 'day' }] }, { value: [] }],
        })
    })
    it('swaps Has related and Has no related, keeping the link and the where', () => {
        const where = TEAM_WITH_SALES.where
        const flipped = where && withPart(CTX, TEAM_WITH_SALES, [0], 'notExists')
        expect(flipped).toEqual({ ...where, op: 'notExists' })
        expect(withPart(CTX, LOOKUP, [0], 'exists')).toEqual({
            op: 'exists',
            link: [{ link: 'sales' }],
        })
    })
})

describe('First match', () => {
    const c: SpecNode = { op: 'case', args: [{ value: true }, { value: 1 }, { value: 2 }] }

    it('adds and removes a case pair before its otherwise', () => {
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
    it('adds and removes its Otherwise', () => {
        expect(hasOtherwise(c)).toBe(true)
        const without = removeOtherwise(c)
        expect(without.args).toEqual([{ value: true }, { value: 1 }])
        expect(hasOtherwise(without)).toBe(false)
        expect(removeOtherwise(without)).toBe(without)
        expect(addOtherwise(without).args).toEqual([{ value: true }, { value: 1 }, { value: null }])
        expect(addOtherwise(c)).toBe(c)
        expect(removeOperandNode(c, 2)).toEqual(without)
        expect(addOperandNode(without).args).toEqual([
            { value: true },
            { value: 1 },
            { value: true },
            { value: null },
        ])
    })
    it('keeps one When/Then pair', () => {
        expect(canRemoveNode(c)).toBe(false)
        expect(removeOperandNode(c, 0)).toBe(c)
        expect(canRemoveNode(addOperandNode(c))).toBe(true)
    })
})

describe('operands', () => {
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

describe('scales follow the operands', () => {
    const div = (top: SpecNode, scale: number): SpecNode => ({
        op: 'div',
        args: [top, { col: 'qty' }],
        scale,
    })

    it('forces whole cents on an amount divided by a number', () => {
        const qtyByQty = div({ col: 'qty' }, 2)
        const back = replaceTreeAt(REPORT_TREE, qtyByQty, [0], { col: 'amount' })
        expect(settleScales(CTX, back, qtyByQty)).toEqual(div({ col: 'amount' }, 0))
        expect(settleScales(CTX, div({ col: 'amount' }, 2))).toEqual(div({ col: 'amount' }, 0))
    })
    it('gives a ratio RATIO_SCALE places when its top stops being an amount', () => {
        const amountByQty = div({ col: 'amount' }, 0)
        const qtyByQty = replaceTreeAt(REPORT_TREE, amountByQty, [0], { col: 'qty' })
        expect(settleScales(CTX, qtyByQty, amountByQty)).toEqual(div({ col: 'qty' }, 4))
        const noScale: SpecNode = { op: 'div', args: [{ col: 'qty' }, { col: 'qty' }] }
        expect(settleScales(CTX, noScale)).toEqual(div({ col: 'qty' }, 4))
    })
    it('keeps the scale an author chose for a ratio through an unrelated edit', () => {
        const stored: SpecNode = { op: 'add', args: [div({ col: 'qty' }, 2), { col: 'amount' }] }
        const edited = replaceTreeAt(REPORT_TREE, stored, [1], { col: 'cost' })
        expect(settleScales(CTX, edited, stored)).toBe(edited)
        const whole = div({ col: 'qty' }, 0)
        expect(settleScales(CTX, whole, whole)).toBe(whole)
    })
    it('keeps an author scale when removing an operand moves its ratio into a forced one', () => {
        const x: SpecNode = { col: 'cost' }
        const stored: SpecNode = {
            op: 'add',
            args: [x, div({ col: 'amount' }, 0), div({ col: 'qty' }, 3)],
        }
        const removed = removeOperandNode(stored, 1)
        expect(removed.args).toEqual([x, div({ col: 'qty' }, 3)])
        expect(settleScales(CTX, removed, stored)).toBe(removed)
    })
    it('averages amounts in whole cents, and gives numbers DEFAULT_PLACES when they stop being amounts', () => {
        const avgQty: SpecNode = { op: 'avg', args: [{ col: 'qty' }], scale: 3 }
        const avgAmount = replaceTreeAt(REPORT_TREE, avgQty, [0], { col: 'amount' })
        expect(settleScales(CTX, avgAmount, avgQty)).toEqual({ ...avgAmount, scale: 0 })
        const forced = { ...avgAmount, scale: 0 }
        const back = replaceTreeAt(REPORT_TREE, forced, [0], { col: 'qty' })
        expect(settleScales(CTX, back, forced)).toEqual({ ...back, scale: 2 })
        expect(settleScales(CTX, avgQty, avgQty)).toBe(avgQty)
    })
    it('keeps a Round as the author set it, and a settled formula as it is', () => {
        const round: SpecNode = { op: 'round', args: [div({ col: 'amount' }, 0)], scale: 3 }
        expect(settleScales(CTX, round)).toBe(round)
        expect(settleScales(CTX, LARGER)).toBe(LARGER)
    })
    it('reads the looked-up source inside a lookup', () => {
        const lookup: SpecNode = {
            op: 'lookup',
            source: SALES_KEY,
            value: { op: 'div', args: [{ path: [{ field: 'amount' }] }, { value: 2 }], scale: 4 },
        }
        expect(settleScales({ ...CTX, source: TEAMS_KEY }, lookup)).toEqual({
            ...lookup,
            value: { op: 'div', args: [{ path: [{ field: 'amount' }] }, { value: 2 }], scale: 0 },
        })
    })
})

describe('what a part offers where it sits', () => {
    it('offers no path, record, param or total in a layout row, but a lookup that reads params', () => {
        const parts = partsAt(LAYOUT, { value: 0 }, [])
        for (const p of ['path', 'record', 'param', 'sum', 'runningSum'])
            expect(parts).not.toContain(p)
        expect(parts).toEqual(expect.arrayContaining(['field', 'lookup']))
        const where = partsAt(LAYOUT, LOOKUP, [0, 1])
        expect(where).toEqual(expect.arrayContaining(['path', 'param', 'field']))
    })
    it('offers every report operator but exists and a list in a column', () => {
        const parts = partsAt(CTX, { value: 0 }, [])
        const offered = [
            'lookup',
            'sum',
            'runningSum',
            'percentOfTotal',
            'case',
            'coalesce',
            'fiscalPeriod',
            'diffDays',
        ]
        for (const p of offered) expect(parts).toContain(p)
        for (const p of ['exists', 'notExists', 'list', 'inPeriod']) expect(parts).not.toContain(p)
    })
    it('offers a list only as the list of Is any of, with a multiple choice param', () => {
        const anyOf: SpecNode = { op: 'in', args: [{ col: 'qty' }, { value: [] }] }
        expect(partsAt(CTX, anyOf, [1])).toEqual(['list'])
        expect(partsAt(WITH_PERIOD, anyOf, [1])).toEqual(['list', 'param'])
        expect(withPart(WITH_PERIOD, anyOf, [1], 'param')).toEqual({ param: 'statuses' })
        expect(partsAt(CTX, anyOf, [0])).not.toContain('list')
    })
    it('offers a param only where one fits', () => {
        const onlyPeriod = { ...CTX, params: [{ id: 'period', type: 'dateRange' as const }] }
        expect(partsAt(onlyPeriod, { value: 0 }, [])).not.toContain('param')
        expect(partsAt(onlyPeriod, { value: 0 }, [])).toContain('inPeriod')
        expect(partsAt(onlyPeriod, IN_PERIOD, [1])).toEqual(['param'])
        expect(partsAt(WITH_PERIOD, IN_PERIOD, [1])).toEqual(['param'])
        expect(withPart(WITH_PERIOD, IN_PERIOD, [1], 'param')).toEqual({ param: 'period' })
    })
    it('offers exists and notExists only inside a lookup', () => {
        expect(partsAt(CTX, LOOKUP, [0])).toEqual(expect.arrayContaining(['exists', 'notExists']))
        expect(partsAt(CTX, LOOKUP, [1])).toEqual(expect.arrayContaining(['exists', 'notExists']))
        expect(partsAt(LAYOUT, { value: 0 }, [])).not.toContain('exists')
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
    it('reads the linked rows inside an exists', () => {
        expect(contextAt(CTX, TEAM_WITH_SALES, [0, 0]).source).toBe(SALES_KEY)
    })
})

describe('what a leaf offers', () => {
    it('lists only date ranges for a Period, multiple choices for a list, and the rest elsewhere', () => {
        const ids = (root: SpecNode, path: number[]) =>
            paramsAt(WITH_PERIOD, root, path).map(p => p.id)
        expect(ids(IN_PERIOD, [1])).toEqual(['period'])
        expect(ids({ op: 'in', args: [{ col: 'qty' }, { value: [] }] }, [1])).toEqual(['statuses'])
        expect(ids({ op: 'eq', args: [{ col: 'qty' }, { value: 0 }] }, [1])).toEqual(['asOf'])
    })
    it('reads a list as numbers when it is tested against a number', () => {
        const of = (tested: SpecNode, list: unknown[]) =>
            isNumberList(CTX, { op: 'in', args: [tested, { value: list }] }, [1])
        expect(of({ col: 'qty' }, [])).toBe(true)
        expect(of({ col: 'amount' }, [])).toBe(true)
        expect(of({ path: [{ field: 'customer' }] }, [])).toBe(false)
        // An untyped value keeps the list's own kind.
        expect(of({ value: null }, [1, 2])).toBe(true)
        expect(of({ value: null }, ['a'])).toBe(false)
    })
    it('says when the engine fixes the places of a part', () => {
        const half: SpecNode = { op: 'div', args: [{ col: 'amount' }, { value: 2 }], scale: 0 }
        expect(forcedScaleAt(CTX, half, [])).toBe(0)
        const ratio: SpecNode = { op: 'div', args: [{ col: 'qty' }, { value: 2 }], scale: 4 }
        expect(forcedScaleAt(CTX, ratio, [])).toBeUndefined()
        const avg: SpecNode = {
            op: 'add',
            args: [{ op: 'avg', args: [{ col: 'amount' }], scale: 0 }, { value: 1 }],
        }
        expect(forcedScaleAt(CTX, avg, [0])).toBe(0)
    })
})

describe('the v3 lookup and leaf parts', () => {
    // Sales on the same day as the report's row (F1 count, F2 outer).
    const SAME_DAY: SpecNode = {
        op: 'lookup',
        source: SALES_KEY,
        where: { op: 'eq', args: [{ path: [{ field: 'day' }] }, { outer: [{ field: 'day' }] }] },
        value: { path: [{ field: 'id' }] },
        pick: 'count',
    }

    it('picks a lookup match as Smallest, Largest, Add up or Count', () => {
        expect(LOOKUP_PICKS).toEqual([
            { value: 'min', label: 'Smallest' },
            { value: 'max', label: 'Largest' },
            { value: 'sum', label: 'Add up' },
            { value: 'count', label: 'Count' },
        ])
        expect(isEditableNode({ ...LOOKUP, pick: 'sum' }, PARAMS)).toBe(true)
        expect(isEditableNode(SAME_DAY, PARAMS)).toBe(true)
    })

    it("offers This row's field only inside a lookup that a report row reads", () => {
        expect(partsAt(CTX, { value: 0 }, [])).not.toContain('outer')
        expect(partsAt(CTX, LOOKUP, [0, 1])).toContain('outer')
        expect(partsAt(CTX, LOOKUP, [1])).toContain('outer')
        // A computed layout row has no report row to read.
        expect(partsAt(LAYOUT, LOOKUP, [0, 1])).not.toContain('outer')
    })

    it("edits This row's field inside a lookup and keeps one outside a lookup read-only", () => {
        expect(partOf({ outer: [{ field: 'day' }] })).toBe('outer')
        expect(isEditableNode({ outer: [{ field: 'day' }] }, PARAMS)).toBe(false)
        expect(
            isEditableNode(
                { op: 'add', args: [{ outer: [{ field: 'amount' }] }, { value: 1 }] },
                PARAMS
            )
        ).toBe(false)
        expect(withPart(CTX, LOOKUP, [0, 1], 'outer')).toEqual({ outer: [{ field: 'customer' }] })
    })

    it('offers a start and an end for each date range param', () => {
        expect(boundChoices(WITH_PERIOD.params)).toEqual([
            { param: 'period', bound: 'from', label: 'Period start' },
            { param: 'period', bound: 'to', label: 'Period end' },
        ])
        expect(boundChoices(CTX.params)).toEqual([])
        expect(partsAt(WITH_PERIOD, { value: 0 }, [])).toContain('bound')
        expect(partsAt(CTX, { value: 0 }, [])).not.toContain('bound')
        // A period slot still takes the range itself, not one of its ends.
        expect(partsAt(WITH_PERIOD, IN_PERIOD, [1])).toEqual(['param'])
        expect(withPart(WITH_PERIOD, { value: 0 }, [], 'bound')).toEqual({
            param: 'period',
            bound: 'from',
        })
        expect(partOf({ param: 'period', bound: 'to' })).toBe('bound')
        expect(isEditableNode({ param: 'period', bound: 'to' }, PARAMS)).toBe(true)
        expect(isEditableNode({ param: 'asOf', bound: 'to' }, PARAMS)).toBe(false)
    })
})
