import { log } from '@tinycld/core/lib/logger'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { SpecNode } from '../../spec'
import { formulaOperands, fromComputeExpr, toComputeExpr } from '../formula'

const amounts = new Set(['amount', 'cost'])
const isAmount = (key: string) => amounts.has(key)

afterEach(() => vi.restoreAllMocks())

describe('the forms formula editor and report formulas', () => {
    it('round-trips arithmetic over columns', () => {
        const node: SpecNode = { op: 'sub', args: [{ col: 'amount' }, { col: 'cost' }] }
        const expr = toComputeExpr(node, 'col')
        expect(expr).toEqual({ sub: [{ var: 'amount' }, { var: 'cost' }] })
        expect(expr === null ? null : fromComputeExpr(expr, 'col', isAmount)).toEqual(node)
    })

    it('reads a division with its scale as Round of a Divide, and back', () => {
        const node: SpecNode = { op: 'div', args: [{ col: 'profit' }, { col: 'amount' }], scale: 2 }
        const expr = toComputeExpr(node, 'col')
        expect(expr).toEqual({ round: [{ div: [{ var: 'profit' }, { var: 'amount' }] }, 2] })
        expect(expr === null ? null : fromComputeExpr(expr, 'col', isAmount)).toEqual(node)
    })

    it('gives a bare Divide scale 0 for an amount over a number, else 4', () => {
        expect(fromComputeExpr({ div: [{ var: 'amount' }, 2] }, 'col', isAmount)).toEqual({
            op: 'div',
            args: [{ col: 'amount' }, { value: 2 }],
            scale: 0,
        })
        expect(
            fromComputeExpr({ div: [{ var: 'cost' }, { var: 'amount' }] }, 'col', isAmount)
        ).toMatchObject({ scale: 4 })
    })

    it('writes Round of anything else as round with a scale', () => {
        expect(
            fromComputeExpr({ round: [{ mul: [{ var: 'amount' }, 1.1] }, 0] }, 'col', isAmount)
        ).toEqual({
            op: 'round',
            args: [{ op: 'mul', args: [{ col: 'amount' }, { value: 1.1 }] }],
            scale: 0,
        })
    })

    it('reads layout rows over subtotals', () => {
        const node: SpecNode = {
            op: 'sub',
            args: [{ subtotal: 'income' }, { subtotal: 'expense' }],
        }
        const expr = toComputeExpr(node, 'subtotal')
        expect(expr).toEqual({ sub: [{ var: 'income' }, { var: 'expense' }] })
        expect(expr === null ? null : fromComputeExpr(expr, 'subtotal', () => true)).toEqual(node)
    })

    it('leaves to the fixed view what the editor cannot show', () => {
        expect(
            toComputeExpr({ op: 'if', args: [{ col: 'a' }, { value: 1 }, { value: 2 }] }, 'col')
        ).toBeNull()
        expect(toComputeExpr({ op: 'sum', args: [{ col: 'amount' }] }, 'col')).toBeNull()
        expect(toComputeExpr({ op: 'max', args: [{ col: 'amount' }] }, 'col')).toBeNull()
        expect(toComputeExpr({ op: 'add', args: [{ col: 'a' }, { param: 'n' }] }, 'col')).toBeNull()
        expect(toComputeExpr({ col: 'amount' }, 'subtotal')).toBeNull()
    })

    it('leaves to the fixed view a name the editor cannot read', () => {
        // The editor reads only lowercase snake_case keys of at most 40 characters.
        expect(
            toComputeExpr({ op: 'sub', args: [{ col: 'netAmount' }, { col: 'cost' }] }, 'col')
        ).toBeNull()
        expect(
            toComputeExpr({ op: 'add', args: [{ subtotal: 'Income' }, { value: 1 }] }, 'subtotal')
        ).toBeNull()
        expect(toComputeExpr({ col: 'a'.repeat(41) }, 'col')).toBeNull()
        expect(toComputeExpr({ col: 'a'.repeat(40) }, 'col')).toEqual({ var: 'a'.repeat(40) })
    })

    it('writes a table aggregate as 0 and says so', () => {
        const warn = vi.spyOn(log, 'warn').mockImplementation(() => {})
        expect(
            fromComputeExpr({ add: [{ sumOf: [{ var: 'lines' }, 'amount'] }, 1] }, 'col', isAmount)
        ).toEqual({
            op: 'add',
            args: [{ value: 0 }, { value: 1 }],
        })
        expect(fromComputeExpr({ count: [{ var: 'lines' }] }, 'col', isAmount)).toEqual({
            value: 0,
        })
        expect(warn).toHaveBeenCalledTimes(2)
        expect(warn).toHaveBeenCalledWith('reports.builder', expect.any(String), expect.anything())
    })

    it('offers the other numeric columns to a column formula', () => {
        const operands = formulaOperands(
            [
                { key: 'customer', label: 'Customer', isNumeric: false },
                { key: 'amount', label: 'Amount', isNumeric: true },
                { key: 'profit', label: 'Profit', isNumeric: true },
                { key: 'netAmount', label: 'Net', isNumeric: true },
            ],
            'profit'
        )
        expect(operands).toEqual({ values: [{ key: 'amount', label: 'Amount' }], tables: [] })
    })
})
