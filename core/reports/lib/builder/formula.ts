import type { FormulaOperands } from '@tinycld/core/lib/forms/builder'
import type { ComputeExpr } from '@tinycld/core/lib/forms/expression'
import { keySchema } from '@tinycld/core/lib/forms/key'
import { log } from '@tinycld/core/lib/logger'
import type { SpecNode } from '../spec'

// The builder edits formulas with the forms engine's FormulaEditor (spec §4), whose
// tree is ComputeExpr. A report formula is the engine's { op, args } tree. These
// two functions translate the arithmetic both share; any other operator, and any
// name the editor cannot read as a key, is not editable here and stays a fixed
// part (R15). A report division always carries a scale: Round of a Divide is a
// division at that scale, and a bare Divide takes whole cents for an amount over
// a number, else RATIO_SCALE places.

export type FormulaLeaf = 'col' | 'subtotal'
export const RATIO_SCALE = 4
const MAX_PLACES = 6
const ARITHMETIC = new Set(['add', 'sub', 'mul', 'div', 'round', 'min', 'max'])

/** The node sets no key outside `keys`; the builder reads only shapes it writes. */
export function onlyKeys(node: SpecNode, keys: readonly string[]): boolean {
    return Object.entries(node).every(([key, value]) => value === undefined || keys.includes(key))
}

const isKey = (name: string) => keySchema.safeParse(name).success

function hasScale(node: SpecNode): node is SpecNode & { scale: number } {
    return node.scale !== undefined && node.scale >= 0 && node.scale <= MAX_PLACES
}

function leafName(node: SpecNode, leaf: FormulaLeaf): string | null {
    const name = leaf === 'col' ? node.col : node.subtotal
    return name && onlyKeys(node, [leaf]) && isKey(name) ? name : null
}

export function toComputeExpr(node: SpecNode, leaf: FormulaLeaf): ComputeExpr | null {
    if (typeof node.value === 'number' && onlyKeys(node, ['value'])) return node.value
    const name = leafName(node, leaf)
    if (name) return { var: name }
    if (!node.op || !ARITHMETIC.has(node.op) || !onlyKeys(node, ['op', 'args', 'scale']))
        return null
    const args: ComputeExpr[] = []
    for (const arg of node.args ?? []) {
        const expr = toComputeExpr(arg, leaf)
        if (expr === null) return null
        args.push(expr)
    }
    const [a, b] = args
    switch (node.op) {
        case 'add':
            return args.length >= 2 ? { add: args } : null
        case 'mul':
            return args.length >= 2 ? { mul: args } : null
        // One argument makes min and max aggregates in a report formula.
        case 'min':
            return args.length >= 2 ? { min: args } : null
        case 'max':
            return args.length >= 2 ? { max: args } : null
        case 'sub':
            return args.length === 2 && a !== undefined && b !== undefined ? { sub: [a, b] } : null
        case 'div':
            return args.length === 2 && a !== undefined && b !== undefined && hasScale(node)
                ? { round: [{ div: [a, b] }, node.scale] }
                : null
        case 'round':
            return args.length === 1 && a !== undefined && hasScale(node)
                ? { round: [a, node.scale] }
                : null
    }
    return null
}

/** The formula's result is an amount, as the engine types it (amount ÷ number stays an amount). */
export function isAmountExpr(expr: ComputeExpr, isAmount: (key: string) => boolean): boolean {
    const amount = (e: ComputeExpr) => isAmountExpr(e, isAmount)
    if (typeof expr === 'number') return false
    if ('var' in expr) return isAmount(expr.var)
    if ('add' in expr) return expr.add.some(amount)
    if ('sub' in expr) return expr.sub.some(amount)
    if ('min' in expr) return expr.min.some(amount)
    if ('max' in expr) return expr.max.some(amount)
    if ('mul' in expr) return expr.mul.filter(amount).length === 1
    if ('div' in expr) return amount(expr.div[0]) && !amount(expr.div[1])
    if ('round' in expr) return amount(expr.round[0])
    return false
}

export function fromComputeExpr(
    expr: ComputeExpr,
    leaf: FormulaLeaf,
    isAmount: (key: string) => boolean
): SpecNode {
    const walk = (e: ComputeExpr): SpecNode => {
        if (typeof e === 'number') return { value: e }
        if ('var' in e) return leaf === 'col' ? { col: e.var } : { subtotal: e.var }
        if ('add' in e) return { op: 'add', args: e.add.map(walk) }
        if ('sub' in e) return { op: 'sub', args: e.sub.map(walk) }
        if ('mul' in e) return { op: 'mul', args: e.mul.map(walk) }
        if ('div' in e) {
            const [top, bottom] = e.div
            const scale =
                isAmountExpr(top, isAmount) && !isAmountExpr(bottom, isAmount) ? 0 : RATIO_SCALE
            return { op: 'div', args: e.div.map(walk), scale }
        }
        if ('round' in e) {
            const [inner, places] = e.round
            if (typeof inner === 'object' && 'div' in inner)
                return { op: 'div', args: inner.div.map(walk), scale: places }
            return { op: 'round', args: [walk(inner)], scale: places }
        }
        if ('min' in e)
            return e.min.length === 1 && e.min[0] !== undefined
                ? walk(e.min[0])
                : { op: 'min', args: e.min.map(walk) }
        if ('max' in e)
            return e.max.length === 1 && e.max[0] !== undefined
                ? walk(e.max[0])
                : { op: 'max', args: e.max.map(walk) }
        // Sum of a column and Number of rows read form tables. formulaOperands
        // offers none, so the editor cannot make one; if one arrives anyway, say so
        // rather than drop it silently.
        log.warn('reports.builder', 'a report formula cannot read a table; it reads 0', { expr: e })
        return { value: 0 }
    }
    return walk(expr)
}

/** What a formula may read: the other numeric choices whose id the editor can read, and no table. */
export function formulaOperands(
    choices: readonly { key: string; label: string; isNumeric: boolean }[],
    selfKey: string
): FormulaOperands {
    return {
        values: choices
            .filter(choice => choice.key !== selfKey && choice.isNumeric && isKey(choice.key))
            .map(choice => ({ key: choice.key, label: choice.label })),
        tables: [],
    }
}
