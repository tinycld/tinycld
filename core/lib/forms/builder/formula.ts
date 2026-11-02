import { type ComputeExpr, computeExprSchema } from '../expression'
import { type FormHostDef, findBuiltin } from '../host'
import { kindOf } from './catalog'
import type { DraftDefinition } from './draft'
import { type FormulaTree, replaceTreeAt, treeAt } from './formula-tree'

/**
 * The builder's structured formula editor works on these pure helpers: a
 * formula is a tree of parts, and every edit replaces one part by its path
 * (operand indexes from the top; Round's one part is index 0). Every helper
 * returns a formula `computeExprSchema` accepts.
 */
export type FormulaPart =
    | 'number'
    | 'field'
    | 'add'
    | 'sub'
    | 'mul'
    | 'div'
    | 'round'
    | 'min'
    | 'max'
    | 'sumOf'
    | 'count'

export const PART_LABELS: Record<FormulaPart, string> = {
    number: 'Number',
    field: 'Field',
    add: 'Add',
    sub: 'Subtract',
    mul: 'Multiply',
    div: 'Divide',
    round: 'Round',
    min: 'Smallest of',
    max: 'Largest of',
    sumOf: 'Sum of a column',
    count: 'Number of rows',
}

const PART_ORDER: FormulaPart[] = [
    'number',
    'field',
    'add',
    'sub',
    'mul',
    'div',
    'round',
    'min',
    'max',
    'sumOf',
    'count',
]

export const FORMULA_PARTS: readonly FormulaPart[] = PART_ORDER

export function isFormulaPart(part: string): part is FormulaPart {
    return (PART_ORDER as readonly string[]).includes(part)
}

type OperatorPart = 'add' | 'sub' | 'mul' | 'div' | 'min' | 'max'
const OPERATORS = new Set<FormulaPart>(['add', 'sub', 'mul', 'div', 'min', 'max'])
const VARIADIC = new Set<FormulaPart>(['add', 'mul', 'min', 'max'])
/** The fewest parts the builder keeps in an operator; the schema allows one in Smallest/Largest of. */
const MIN_PARTS = 2
const ROUND_PLACES = 2

export interface FormulaChoice {
    key: string
    label: string
}

export interface FormulaTable extends FormulaChoice {
    /** Its number and calculated columns: the ones Sum of a column can add up. */
    columns: FormulaChoice[]
}

/** What a formula can read; see `fieldOperands` and `columnOperands`. */
export interface FormulaOperands {
    values: FormulaChoice[]
    tables: FormulaTable[]
}

export type FormulaPath = readonly number[]

/** A draft holds its formula untyped; anything that is not a formula reads as 0. */
export function asExpr(value: unknown): ComputeExpr {
    const parsed = computeExprSchema.safeParse(value)
    return parsed.success ? parsed.data : 0
}

export function partOf(expr: ComputeExpr): FormulaPart {
    if (typeof expr === 'number') return 'number'
    if ('var' in expr) return 'field'
    if ('add' in expr) return 'add'
    if ('sub' in expr) return 'sub'
    if ('mul' in expr) return 'mul'
    if ('div' in expr) return 'div'
    if ('round' in expr) return 'round'
    if ('min' in expr) return 'min'
    if ('max' in expr) return 'max'
    if ('sumOf' in expr) return 'sumOf'
    return 'count'
}

/** The parts a part holds, in order: an operator's operands, or the one part Round rounds. */
export function childrenOf(expr: ComputeExpr): ComputeExpr[] {
    if (typeof expr === 'number' || 'var' in expr || 'sumOf' in expr || 'count' in expr) return []
    if ('add' in expr) return expr.add
    if ('sub' in expr) return expr.sub
    if ('mul' in expr) return expr.mul
    if ('div' in expr) return expr.div
    if ('round' in expr) return [expr.round[0]]
    if ('min' in expr) return expr.min
    return expr.max
}

const isOperatorPart = (part: FormulaPart): part is OperatorPart => OPERATORS.has(part)

function operatorNode(part: OperatorPart, parts: ComputeExpr[]): ComputeExpr {
    const [a = 0, b = 0] = parts
    switch (part) {
        case 'add':
            return { add: parts }
        case 'sub':
            return { sub: [a, b] }
        case 'mul':
            return { mul: parts }
        case 'div':
            return { div: [a, b] }
        case 'min':
            return { min: parts }
        case 'max':
            return { max: parts }
    }
}

export function withChildren(expr: ComputeExpr, parts: ComputeExpr[]): ComputeExpr {
    const part = partOf(expr)
    if (isOperatorPart(part)) return operatorNode(part, parts)
    if (typeof expr === 'object' && 'round' in expr)
        return { round: [parts[0] ?? 0, expr.round[1]] }
    return expr
}

const COMPUTE_TREE: FormulaTree<ComputeExpr> = { childrenOf, withChildren }

export const exprAt = (expr: ComputeExpr, path: FormulaPath) => treeAt(COMPUTE_TREE, expr, path)

/** The formula with the part at `path` replaced; an unknown path changes nothing. */
export const replaceAt = (expr: ComputeExpr, path: FormulaPath, next: ComputeExpr) =>
    replaceTreeAt(COMPUTE_TREE, expr, path, next)

const padTo = (parts: ComputeExpr[], n: number): ComputeExpr[] => [
    ...parts,
    ...Array.from({ length: Math.max(0, n - parts.length) }, () => 0),
]

function tableOf(expr: ComputeExpr, operands: FormulaOperands): FormulaTable | undefined {
    if (typeof expr !== 'object') return undefined
    const key = 'sumOf' in expr ? expr.sumOf[0].var : 'count' in expr ? expr.count[0].var : null
    return operands.tables.find(t => t.key === key)
}

/**
 * The part changed to another kind of part. An operator's parts carry over
 * (Subtract and Divide keep the first two); any other part becomes the first
 * part of the new operator. Round wraps the current part. A part with nothing
 * to read (Field with no values, a total with no table) changes nothing.
 */
export function withPart(
    expr: ComputeExpr,
    part: FormulaPart,
    operands: FormulaOperands
): ComputeExpr {
    if (part === partOf(expr)) return expr
    const seed = isOperatorPart(partOf(expr)) ? childrenOf(expr) : [expr]
    switch (part) {
        case 'number':
            return 0
        case 'field': {
            const first = operands.values[0]
            return first ? { var: first.key } : expr
        }
        case 'sub':
        case 'div':
            return operatorNode(part, padTo(seed, MIN_PARTS).slice(0, 2))
        case 'add':
        case 'mul':
        case 'min':
        case 'max':
            return operatorNode(part, padTo(seed, MIN_PARTS))
        case 'round':
            return { round: [expr, ROUND_PLACES] }
        case 'sumOf': {
            const kept = tableOf(expr, operands)
            const table = kept?.columns.length ? kept : operands.tables.find(t => t.columns.length)
            const column = table?.columns[0]
            return table && column ? { sumOf: [{ var: table.key }, column.key] } : expr
        }
        case 'count': {
            const table = tableOf(expr, operands) ?? operands.tables[0]
            return table ? { count: [{ var: table.key }] } : expr
        }
    }
}

export const isVariadic = (expr: ComputeExpr) => VARIADIC.has(partOf(expr))

export const canRemoveOperand = (expr: ComputeExpr) =>
    isVariadic(expr) && childrenOf(expr).length > MIN_PARTS

export function addOperand(expr: ComputeExpr): ComputeExpr {
    if (!isVariadic(expr)) return expr
    return withChildren(expr, [...childrenOf(expr), 0])
}

export function removeOperand(expr: ComputeExpr, index: number): ComputeExpr {
    if (!canRemoveOperand(expr)) return expr
    return withChildren(
        expr,
        childrenOf(expr).filter((_, i) => i !== index)
    )
}

export function setPlaces(expr: ComputeExpr, places: number): ComputeExpr {
    if (typeof expr !== 'object' || !('round' in expr)) return expr
    return { round: [expr.round[0], places] }
}

/** Points a total at another table; Sum of a column keeps its column when that table has it. */
export function setAggregateTable(
    expr: ComputeExpr,
    key: string,
    operands: FormulaOperands
): ComputeExpr {
    const table = operands.tables.find(t => t.key === key)
    if (!table || typeof expr !== 'object') return expr
    if ('count' in expr) return { count: [{ var: key }] }
    if (!('sumOf' in expr)) return expr
    const column = table.columns.find(c => c.key === expr.sumOf[1]) ?? table.columns[0]
    return column ? { sumOf: [{ var: key }, column.key] } : expr
}

export function setAggregateColumn(expr: ComputeExpr, column: string): ComputeExpr {
    if (typeof expr !== 'object' || !('sumOf' in expr)) return expr
    return { sumOf: [expr.sumOf[0], column] }
}

/** The parts a menu offers: Field needs a value to read, the totals a table. */
export function partsFor(operands: FormulaOperands): FormulaPart[] {
    return PART_ORDER.filter(part => {
        if (part === 'field') return operands.values.length > 0
        if (part === 'sumOf') return operands.tables.some(t => t.columns.length > 0)
        if (part === 'count') return operands.tables.length > 0
        return true
    })
}

/** The formula as one line of text, by label: "Qty × Price", "sum of Total in Parts". */
export function describeFormula(expr: ComputeExpr, operands: FormulaOperands): string {
    const missing = (key: string) => `${key} (missing)`
    const valueLabel = (key: string) =>
        operands.values.find(v => v.key === key)?.label ?? missing(key)
    const tableLabel = (key: string) =>
        operands.tables.find(t => t.key === key)?.label ?? missing(key)
    const columnLabel = (table: string, key: string) =>
        operands.tables.find(t => t.key === table)?.columns.find(c => c.key === key)?.label ??
        missing(key)
    const say = (e: ComputeExpr, isNested: boolean): string => {
        const join = (parts: ComputeExpr[], sign: string) => {
            const text = parts.map(p => say(p, true)).join(sign)
            return isNested ? `(${text})` : text
        }
        const list = (parts: ComputeExpr[]) => parts.map(p => say(p, false)).join(', ')
        if (typeof e === 'number') return String(e)
        if ('var' in e) return valueLabel(e.var)
        if ('add' in e) return join(e.add, ' + ')
        if ('sub' in e) return join(e.sub, ' − ')
        if ('mul' in e) return join(e.mul, ' × ')
        if ('div' in e) return join(e.div, ' ÷ ')
        if ('round' in e) return `round(${say(e.round[0], false)}, ${e.round[1]})`
        if ('min' in e) return `smallest of (${list(e.min)})`
        if ('max' in e) return `largest of (${list(e.max)})`
        if ('sumOf' in e) {
            const [{ var: table }, column] = e.sumOf
            return `sum of ${columnLabel(table, column)} in ${tableLabel(table)}`
        }
        return `rows in ${tableLabel(e.count[0].var)}`
    }
    return say(expr, false)
}

/** Mirrors computedIssues: what a formula may read, and what a total may add up. */
const READABLE_KINDS = new Set(['number', 'computed', 'toggle'])
const SUMMABLE_KINDS = new Set(['number', 'computed'])

type DraftField = DraftDefinition['fields'][number]

interface ColumnLike {
    key: string
    label: string
    kind: string
}

const choiceOf = (c: { key: string; label: string }): FormulaChoice => ({
    key: c.key,
    label: c.label,
})

function columnsOf(field: DraftField, host: FormHostDef): readonly ColumnLike[] {
    if (field.kind === 'table') return field.columns
    if (field.kind === 'builtin') return findBuiltin(host, field.key)?.columns ?? []
    return []
}

/** What a calculated value's formula can read: every other numeric field, and each table. */
export function fieldOperands(
    draft: DraftDefinition,
    host: FormHostDef,
    selfKey: string
): FormulaOperands {
    const values: FormulaChoice[] = []
    const tables: FormulaTable[] = []
    for (const field of draft.fields) {
        if (field.key === selfKey) continue
        const kind = kindOf(draft, host, field.key)
        if (kind && READABLE_KINDS.has(kind)) values.push(choiceOf(field))
        if (kind === 'table') {
            const columns = columnsOf(field, host)
                .filter(c => SUMMABLE_KINDS.has(c.kind))
                .map(choiceOf)
            tables.push({ ...choiceOf(field), columns })
        }
    }
    return { values, tables }
}

/** What a column's formula can read: the row's other numeric columns. A row cannot total a table. */
export function columnOperands(columns: readonly ColumnLike[], selfKey: string): FormulaOperands {
    const values = columns
        .filter(c => c.key !== selfKey && READABLE_KINDS.has(c.kind))
        .map(choiceOf)
    return { values, tables: [] }
}
