import type { FormulaTree } from '@tinycld/core/lib/forms/builder/formula-tree'
import { type ParamDef, type SpecNode, specNodeSchema } from '../spec'
import type { FieldType } from '../types'
import type { Catalog } from './columns'
import { onlyKeys } from './formula'

// The report builder's formula parts (OQ2): every operator the engine compiles
// (core/server/reports/formula.go), each named by a part id. The parts the forms
// editor also has keep its ids (number, field, add, sub, mul, div, round, min, max),
// so their test ids stay (R16). A formula draft is the spec's own { op, args } tree
// (R17). Three parts are an operator in one shape: smallest and largest are min and
// max of one value (a total), and inPeriod is between a value and a date range
// param. exists and notExists are parts only inside a lookup; elsewhere the filter
// editor owns them. This module names the parts and reads a node's part; the
// editing operations are in formula-ops.ts, types in formula-type.ts and the
// read-back line in formula-describe.ts.

const VALUE_PARTS = [
    'number',
    'text',
    'yesno',
    'empty',
    'list',
    'field',
    'path',
    'record',
    'param',
] as const
const ARITHMETIC = ['add', 'sub', 'mul', 'div', 'round', 'min', 'max'] as const
const TOTALS = ['sum', 'count', 'countDistinct', 'avg', 'smallest', 'largest'] as const
const WINDOWS = ['runningSum', 'percentOfParent', 'percentOfTotal'] as const
const LOGIC = [
    'eq',
    'ne',
    'lt',
    'lte',
    'gt',
    'gte',
    'between',
    'inPeriod',
    'in',
    'and',
    'or',
    'not',
    'isEmpty',
    'if',
    'case',
    'coalesce',
] as const
const TEXT = ['concat', 'upper', 'lower', 'contains', 'startsWith'] as const
const DATES = [
    'year',
    'quarter',
    'month',
    'fiscalYear',
    'fiscalPeriod',
    'addDays',
    'addMonths',
    'diffDays',
] as const
const RELATED = ['exists', 'notExists'] as const
export const REPORT_PARTS = [
    ...VALUE_PARTS,
    ...ARITHMETIC,
    ...TOTALS,
    ...WINDOWS,
    ...LOGIC,
    ...TEXT,
    ...DATES,
    ...RELATED,
    'lookup',
] as const
export type ReportPart = (typeof REPORT_PARTS)[number]

export const VALUE_SET: ReadonlySet<string> = new Set(VALUE_PARTS)
export const TOTAL_SET: ReadonlySet<string> = new Set(TOTALS)
export const WINDOW_SET: ReadonlySet<string> = new Set(WINDOWS)
export const RELATED_SET: ReadonlySet<string> = new Set(RELATED)
export const SCALED_PARTS: ReadonlySet<string> = new Set([
    'div',
    'round',
    'avg',
    'percentOfParent',
    'percentOfTotal',
])
/** The operator a part writes when it is not the part's own id. */
export const OP_OF: Partial<Record<ReportPart, string>> = {
    smallest: 'min',
    largest: 'max',
    inPeriod: 'between',
}

export const INF = Number.POSITIVE_INFINITY
type Arity = readonly [number, number]
// How many operands each operator takes (formula.go operatorExpr's argc).
export const ARITY: Readonly<Record<string, Arity>> = {
    add: [2, INF],
    sub: [2, 2],
    mul: [2, INF],
    div: [2, 2],
    round: [1, 1],
    min: [2, INF],
    max: [2, INF],
    sum: [1, 1],
    count: [1, 1],
    countDistinct: [1, 1],
    avg: [1, 1],
    smallest: [1, 1],
    largest: [1, 1],
    runningSum: [1, 1],
    percentOfParent: [1, 1],
    percentOfTotal: [1, 1],
    eq: [2, 2],
    ne: [2, 2],
    lt: [2, 2],
    lte: [2, 2],
    gt: [2, 2],
    gte: [2, 2],
    between: [3, 3],
    inPeriod: [2, 2],
    in: [2, 2],
    and: [2, INF],
    or: [2, INF],
    not: [1, 1],
    isEmpty: [1, 1],
    if: [3, 3],
    case: [2, INF],
    coalesce: [1, INF],
    concat: [1, INF],
    upper: [1, 1],
    lower: [1, 1],
    contains: [2, 2],
    startsWith: [2, 2],
    year: [1, 1],
    quarter: [1, 1],
    month: [1, 1],
    fiscalYear: [1, 1],
    fiscalPeriod: [1, 1],
    addDays: [2, 2],
    addMonths: [2, 2],
    diffDays: [2, 2],
}

export const PART_LABELS: Record<ReportPart, string> = {
    number: 'Number',
    text: 'Text',
    yesno: 'Yes or no',
    empty: 'Empty',
    list: 'List',
    field: 'Column',
    path: 'Field',
    record: 'Group field',
    param: 'Asked value',
    add: 'Add',
    sub: 'Subtract',
    mul: 'Multiply',
    div: 'Divide',
    round: 'Round',
    min: 'Smallest of',
    max: 'Largest of',
    sum: 'Sum of',
    count: 'Count of',
    countDistinct: 'Count distinct of',
    avg: 'Average of',
    smallest: 'Smallest value of',
    largest: 'Largest value of',
    runningSum: 'Running sum',
    percentOfParent: 'Percent of group',
    percentOfTotal: 'Percent of total',
    eq: 'Is equal',
    ne: 'Is not equal',
    lt: 'Is less than',
    lte: 'Is at most',
    gt: 'Is more than',
    gte: 'Is at least',
    between: 'Is between',
    inPeriod: 'Is in period',
    in: 'Is any of',
    and: 'All of',
    or: 'Any of',
    not: 'Not',
    isEmpty: 'Is empty',
    if: 'If',
    case: 'First match',
    coalesce: 'First not empty',
    concat: 'Join text',
    upper: 'Upper case',
    lower: 'Lower case',
    contains: 'Contains',
    startsWith: 'Starts with',
    year: 'Year of',
    quarter: 'Quarter of',
    month: 'Month of',
    fiscalYear: 'Fiscal year of',
    fiscalPeriod: 'Fiscal period of',
    addDays: 'Add days',
    addMonths: 'Add months',
    diffDays: 'Days between',
    exists: 'Has related',
    notExists: 'Has no related',
    lookup: 'Look up',
}

export interface FormulaValue {
    key: string
    label: string
    type: FieldType
}
export interface FormulaRecord {
    group: string
    label: string
    source: string
}
export type FormulaParam = Pick<ParamDef, 'id' | 'label' | 'type' | 'multiple'>

/** What a formula may read where it is edited. */
export interface ReportFormulaContext {
    catalog: Catalog
    /** {col} in a column, label, sort or having; {subtotal} in a layout row. */
    leaf: 'col' | 'subtotal'
    values: readonly FormulaValue[]
    /** The source a {path} reads from; null where no path is read. */
    source: string | null
    records: readonly FormulaRecord[]
    params: readonly FormulaParam[]
    /**
     * The params a lookup inside reads, where the formula itself reads none: a layout
     * computed row (layout.go param refuses one; lookup.go lookupCtx.param reads it).
     */
    lookupParams?: readonly FormulaParam[]
    totals: boolean
    windows: boolean
    lookups: boolean
    /** Inside a lookup (set by contextAt): exists and notExists read the looked-up rows' links. */
    inLookup?: boolean
}

export const isReportPart = (part: string): part is ReportPart =>
    (REPORT_PARTS as readonly string[]).includes(part)

// A date range param is read only by between (formula.go between); every other
// operator refuses it.
export const isRangeParam = (param: Pick<ParamDef, 'type'>) => param.type === 'dateRange'

/** A lookup's value is a formula over the looked-up source, stored in its `value`. */
export function lookupValueNode(node: SpecNode): SpecNode {
    const parsed = specNodeSchema.safeParse(node.value)
    return parsed.success ? parsed.data : { value: 0 }
}

const ALWAYS: SpecNode = { value: true }
const isAlways = (node: SpecNode | undefined) =>
    node !== undefined && node.value === true && onlyKeys(node, ['value'])

// A where left out matches every row, as a where of yes does; one left out stays
// out while its slot still holds that yes, so an untouched formula is saved as it
// was read.
function withWhere(node: SpecNode, where: SpecNode | undefined): SpecNode {
    if (where === undefined || (node.where === undefined && isAlways(where))) return node
    return { ...node, where }
}

export const REPORT_TREE: FormulaTree<SpecNode> = {
    childrenOf: node => {
        if (node.op === 'lookup') return [node.where ?? ALWAYS, lookupValueNode(node)]
        if (node.op === 'exists' || node.op === 'notExists') return [node.where ?? ALWAYS]
        return node.op ? (node.args ?? []) : []
    },
    withChildren: (node, parts) => {
        if (node.op === 'lookup') return { ...withWhere(node, parts[0]), value: parts[1] }
        if (node.op === 'exists' || node.op === 'notExists') return withWhere(node, parts[0])
        return node.op ? { ...node, args: parts } : node
    },
}

function valuePartOf(value: unknown): ReportPart | 'unknown' {
    if (value === null) return 'empty'
    if (typeof value === 'number') return 'number'
    if (typeof value === 'string') return 'text'
    if (typeof value === 'boolean') return 'yesno'
    if (Array.isArray(value)) return 'list'
    return 'unknown'
}

function operatorPartOf(node: SpecNode): ReportPart | 'unknown' {
    const op = node.op ?? ''
    const n = node.args?.length ?? 0
    if (op === 'min' && n === 1) return 'smallest'
    if (op === 'max' && n === 1) return 'largest'
    if (op === 'between' && n === 2) return 'inPeriod'
    if (!isReportPart(op) || VALUE_SET.has(op) || OP_OF[op] !== undefined) return 'unknown'
    return op
}

export function partOf(node: SpecNode): ReportPart | 'unknown' {
    if (node.record !== undefined) return 'record'
    if (node.col !== undefined || node.subtotal !== undefined) return 'field'
    if (node.path !== undefined) return 'path'
    if (node.param !== undefined) return 'param'
    if (node.op === undefined) return valuePartOf(node.value)
    return operatorPartOf(node)
}
