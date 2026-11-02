import type { FormulaPath } from '@tinycld/core/lib/forms/builder/formula'
import { type FormulaTree, treeAt } from '@tinycld/core/lib/forms/builder/formula-tree'
import { type ParamDef, type SpecHop, type SpecNode, specNodeSchema } from '../spec'
import type { FieldType, ParamType } from '../types'
import { type Catalog, describePath, firstTextField, pathLabel } from './columns'
import { onlyKeys } from './formula'

// The report builder's formula parts (OQ2): every operator the engine compiles
// (core/server/reports/formula.go) except exists and notExists, which the filter
// editor owns. The parts the forms editor also has keep its ids (number, field, add,
// sub, mul, div, round, min, max), so their test ids stay (R16). A formula draft is
// the spec's own { op, args } tree (R17). Three parts are an operator in one shape:
// smallest and largest are min and max of one value (a total), and inPeriod is
// between a value and a date range param.

export const RATIO_SCALE = 4
const DEFAULT_PLACES = 2
const MAX_SCALE = 6

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
export const REPORT_PARTS = [
    ...VALUE_PARTS,
    ...ARITHMETIC,
    ...TOTALS,
    ...WINDOWS,
    ...LOGIC,
    ...TEXT,
    ...DATES,
    'lookup',
] as const
export type ReportPart = (typeof REPORT_PARTS)[number]

const VALUE_SET: ReadonlySet<string> = new Set(VALUE_PARTS)
const TOTAL_SET: ReadonlySet<string> = new Set(TOTALS)
const WINDOW_SET: ReadonlySet<string> = new Set(WINDOWS)
export const SCALED_PARTS: ReadonlySet<string> = new Set([
    'div',
    'round',
    'avg',
    'percentOfParent',
    'percentOfTotal',
])
/** The operator a part writes when it is not the part's own id. */
const OP_OF: Partial<Record<ReportPart, string>> = {
    smallest: 'min',
    largest: 'max',
    inPeriod: 'between',
}

const INF = Number.POSITIVE_INFINITY
type Arity = readonly [number, number]
// How many operands each operator takes (formula.go operatorExpr's argc).
const ARITY: Readonly<Record<string, Arity>> = {
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

/** What a formula may read where it is edited. */
export interface ReportFormulaContext {
    catalog: Catalog
    /** {col} in a column, label, sort or having; {subtotal} in a layout row. */
    leaf: 'col' | 'subtotal'
    values: readonly FormulaValue[]
    /** The source a {path} reads from; null where no path is read. */
    source: string | null
    records: readonly FormulaRecord[]
    params: readonly Pick<ParamDef, 'id' | 'label' | 'type'>[]
    totals: boolean
    windows: boolean
    lookups: boolean
}

export const isReportPart = (part: string): part is ReportPart =>
    (REPORT_PARTS as readonly string[]).includes(part)

// A date range param is read only by between (formula.go between); every other
// operator refuses it.
const isRangeParam = (param: Pick<ParamDef, 'type'>) => param.type === 'dateRange'

/** A lookup's value is a formula over the looked-up source, stored in its `value`. */
function lookupValueNode(node: SpecNode): SpecNode {
    const parsed = specNodeSchema.safeParse(node.value)
    return parsed.success ? parsed.data : { value: 0 }
}

export const REPORT_TREE: FormulaTree<SpecNode> = {
    childrenOf: node => {
        if (node.op === 'lookup') return [node.where ?? { value: true }, lookupValueNode(node)]
        return node.op ? (node.args ?? []) : []
    },
    withChildren: (node, parts) => {
        if (node.op === 'lookup') return { ...node, where: parts[0], value: parts[1] }
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

// A value path the editor writes: to-one links by key, ending at a field or a
// link (bind.go path, pathValue). A hop with any other key (with, mode) stays as
// it is, read-only.
const isFieldPath = (path: readonly SpecHop[]) =>
    path.length > 0 &&
    path.every((hop, i) => {
        const keys = Object.entries(hop).filter(([, value]) => value !== undefined)
        const only = (key: keyof SpecHop) => keys.length === 1 && keys[0]?.[0] === key
        return only('link') || (i === path.length - 1 && only('field'))
    })

// A list of texts or of numbers, as `in` compares them (formula.go literal).
const isValueList = (value: unknown) =>
    Array.isArray(value) &&
    (value.every(v => typeof v === 'string') || value.every(v => typeof v === 'number'))

const hasScale = (node: SpecNode) =>
    node.scale !== undefined && node.scale >= 0 && node.scale <= MAX_SCALE

/** The node has only shapes this editor writes; anything else stays read-only (R17). */
export function isEditableNode(node: SpecNode): boolean {
    const part = partOf(node)
    switch (part) {
        case 'unknown':
            return false
        case 'number':
        case 'text':
        case 'yesno':
        case 'empty':
            return onlyKeys(node, ['value'])
        case 'list':
            return onlyKeys(node, ['value']) && isValueList(node.value)
        case 'field':
            return node.col !== undefined ? onlyKeys(node, ['col']) : onlyKeys(node, ['subtotal'])
        case 'path':
            return onlyKeys(node, ['path']) && isFieldPath(node.path ?? [])
        case 'record':
            return onlyKeys(node, ['record', 'path']) && isFieldPath(node.path ?? [])
        case 'param':
            return onlyKeys(node, ['param'])
        case 'lookup':
            return (
                onlyKeys(node, ['op', 'source', 'where', 'value', 'pick']) &&
                typeof node.source === 'string' &&
                specNodeSchema.safeParse(node.value).success &&
                REPORT_TREE.childrenOf(node).every(isEditableNode)
            )
    }
    const [min, max] = ARITY[part] ?? [0, 0]
    const args = node.args ?? []
    const scaleOk = SCALED_PARTS.has(part) ? hasScale(node) : node.scale === undefined
    const shapeOk = part !== 'inPeriod' || (args[1] !== undefined && partOf(args[1]) === 'param')
    return (
        onlyKeys(node, ['op', 'args', 'scale']) &&
        args.length >= min &&
        args.length <= max &&
        scaleOk &&
        shapeOk &&
        args.every(isEditableNode)
    )
}

/** What may be read at `path`: inside a lookup its source and no totals; inside a total no total. */
export function contextAt(
    ctx: ReportFormulaContext,
    root: SpecNode,
    path: FormulaPath
): ReportFormulaContext {
    let here = ctx
    let node: SpecNode | undefined = root
    for (const index of path) {
        if (!node) break
        const part = partOf(node)
        if (part === 'lookup')
            here = { ...here, source: node.source ?? here.source, totals: false, windows: false }
        else if (TOTAL_SET.has(part)) here = { ...here, totals: false, windows: false }
        else if (WINDOW_SET.has(part)) here = { ...here, windows: false }
        node = REPORT_TREE.childrenOf(node)[index]
    }
    return here
}

export function partsAt(
    ctx: ReportFormulaContext,
    root: SpecNode,
    path: FormulaPath
): ReportPart[] {
    const here = contextAt(ctx, root, path)
    return REPORT_PARTS.filter(part => {
        if (part === 'field') return here.values.length > 0
        if (part === 'path') return here.source !== null
        if (part === 'record') return here.records.length > 0
        if (part === 'param') return here.params.length > 0
        if (part === 'inPeriod') return here.params.some(isRangeParam)
        if (part === 'lookup') return here.lookups && here.catalog.size > 0
        if (TOTAL_SET.has(part)) return here.totals
        if (WINDOW_SET.has(part)) return here.windows
        return true
    })
}

export function partLabel(ctx: ReportFormulaContext, part: string): string {
    if (part === 'field') return ctx.leaf === 'col' ? 'Column' : 'Section'
    return isReportPart(part) ? PART_LABELS[part] : part
}

function firstFieldOf(ctx: ReportFormulaContext, sourceKey: string | null): string | null {
    const source = sourceKey ? ctx.catalog.get(sourceKey) : undefined
    return source?.fields.find(f => f.key !== 'id')?.key ?? source?.fields[0]?.key ?? null
}

// What a new operand slot holds: a yes for All/Any of, an empty text for Join
// text, an empty value for First not empty, else 0.
const FILLER: Partial<Record<ReportPart, SpecNode>> = {
    and: { value: true },
    or: { value: true },
    concat: { value: '' },
    coalesce: { value: null },
}

function seedOf(current: SpecNode): SpecNode[] {
    const part = partOf(current)
    const isOperator = !VALUE_SET.has(part) && part !== 'lookup' && part !== 'unknown'
    return isOperator ? REPORT_TREE.childrenOf(current) : [current]
}

function leafPart(
    here: ReportFormulaContext,
    current: SpecNode,
    part: ReportPart
): SpecNode | undefined {
    switch (part) {
        case 'number':
            return { value: 0 }
        case 'text':
            return { value: '' }
        case 'yesno':
            return { value: true }
        case 'empty':
            return { value: null }
        case 'list':
            return { value: [] }
        case 'field': {
            const first = here.values[0]
            if (!first) return current
            return here.leaf === 'col' ? { col: first.key } : { subtotal: first.key }
        }
        case 'path': {
            const field = firstFieldOf(here, here.source)
            return field ? { path: [{ field }] } : current
        }
        case 'record': {
            const record = here.records[0]
            const source = record ? here.catalog.get(record.source) : undefined
            const field = source
                ? (firstTextField(source)?.key ?? firstFieldOf(here, source.key))
                : null
            return record && field ? { record: record.group, path: [{ field }] } : current
        }
        case 'param': {
            const param = here.params.find(p => !isRangeParam(p)) ?? here.params[0]
            return param ? { param: param.id } : current
        }
        case 'lookup': {
            const source = here.catalog.keys().next().value
            if (!source) return current
            return {
                op: 'lookup',
                source,
                where: { value: true },
                value: { path: [{ field: 'id' }] },
                pick: 'min',
            }
        }
    }
    return undefined
}

/**
 * The part at `path` changed to another kind: returns the replacement for that part
 * (not a new root), operands carried over where they can be.
 */
export function withPart(
    ctx: ReportFormulaContext,
    root: SpecNode,
    path: FormulaPath,
    part: ReportPart
): SpecNode {
    const current = treeAt(REPORT_TREE, root, path) ?? { value: 0 }
    if (partOf(current) === part) return current
    const here = contextAt(ctx, root, path)
    const leaf = leafPart(here, current, part)
    if (leaf) return leaf
    switch (part) {
        case 'if':
            return { op: 'if', args: [{ value: true }, current, { value: null }] }
        case 'case':
            return { op: 'case', args: [{ value: true }, current] }
        case 'in':
            return { op: 'in', args: [current, { value: [] }] }
        case 'inPeriod': {
            const period = here.params.find(isRangeParam)
            return period ? { op: 'between', args: [current, { param: period.id }] } : current
        }
    }
    const [min, max] = ARITY[part] ?? [1, 1]
    const seed = max === 1 ? [current] : seedOf(current)
    const filler = FILLER[part] ?? { value: 0 }
    const padding = Array.from({ length: Math.max(0, min - seed.length) }, () => filler)
    const args = [...seed, ...padding].slice(0, max)
    const node: SpecNode = { op: OP_OF[part] ?? part, args }
    if (part === 'div') node.scale = divScale(here, args)
    else if (part === 'avg') node.scale = isAmount(here, args[0]) ? 0 : DEFAULT_PLACES
    else if (SCALED_PARTS.has(part)) node.scale = DEFAULT_PLACES
    return node
}

function typesOf(ctx: ReportFormulaContext): FormulaTypes {
    return {
        catalog: ctx.catalog,
        source: ctx.source,
        col: key => ctx.values.find(v => v.key === key)?.type,
        records: ctx.records,
        params: ctx.params,
    }
}

const isAmount = (ctx: ReportFormulaContext, node: SpecNode | undefined) =>
    node !== undefined && formulaType(node, typesOf(ctx)) === 'money'

// An amount divided by a number keeps whole cents (the engine refuses another
// scale); so does an average of amounts (withPart).
function divScale(ctx: ReportFormulaContext, args: SpecNode[]): number {
    const [top, bottom] = args
    return isAmount(ctx, top) && !isAmount(ctx, bottom) ? 0 : RATIO_SCALE
}

export function slotLabel(node: SpecNode, index: number): string | null {
    const n = node.args?.length ?? 0
    switch (partOf(node)) {
        case 'if':
            return ['When', 'Then', 'Otherwise'][index] ?? null
        case 'case':
            if (n % 2 === 1 && index === n - 1) return 'Otherwise'
            return index % 2 === 0 ? 'When' : 'Then'
        case 'lookup':
            return ['Where', 'Value'][index] ?? null
        case 'between':
            return ['Value', 'From', 'To'][index] ?? null
        case 'inPeriod':
            return ['Value', 'Period'][index] ?? null
        case 'in':
            return ['Value', 'Is any of'][index] ?? null
        case 'addDays':
        case 'addMonths':
            return ['Date', 'By'][index] ?? null
        case 'diffDays':
            return ['From', 'To'][index] ?? null
    }
    return null
}

export function isVariadicNode(node: SpecNode): boolean {
    return (ARITY[partOf(node)]?.[1] ?? 0) === INF
}

export function canRemoveNode(node: SpecNode): boolean {
    const part = partOf(node)
    const n = node.args?.length ?? 0
    if (part === 'case') return n >= 4
    return isVariadicNode(node) && n > (ARITY[part]?.[0] ?? 0)
}

/** Add part: a case gains a When/Then pair before its Otherwise; others a filler (FILLER). */
export function addOperandNode(node: SpecNode): SpecNode {
    if (!isVariadicNode(node)) return node
    const args = node.args ?? []
    const part = partOf(node)
    if (part === 'case') {
        const pair: SpecNode[] = [{ value: true }, { value: null }]
        const hasElse = args.length % 2 === 1
        const next = hasElse
            ? [...args.slice(0, -1), ...pair, ...args.slice(-1)]
            : [...args, ...pair]
        return { ...node, args: next }
    }
    const filler = part === 'unknown' ? undefined : FILLER[part]
    return { ...node, args: [...args, filler ?? { value: 0 }] }
}

/** Remove part: a case loses the When/Then pair the operand belongs to. */
export function removeOperandNode(node: SpecNode, index: number): SpecNode {
    if (!canRemoveNode(node)) return node
    const args = node.args ?? []
    if (partOf(node) === 'case') {
        const start = index - (index % 2)
        return { ...node, args: args.filter((_, i) => i !== start && i !== start + 1) }
    }
    return { ...node, args: args.filter((_, i) => i !== index) }
}

const SIGNS: Partial<Record<string, string>> = {
    add: ' + ',
    sub: ' − ',
    mul: ' × ',
    div: ' ÷ ',
    eq: ' = ',
    ne: ' ≠ ',
    lt: ' < ',
    lte: ' ≤ ',
    gt: ' > ',
    gte: ' ≥ ',
}
const WORDS: Partial<Record<string, string>> = {
    sum: 'sum of',
    count: 'count of',
    countDistinct: 'distinct count of',
    avg: 'average of',
    smallest: 'smallest',
    largest: 'largest',
    runningSum: 'running sum of',
    upper: 'upper case of',
    lower: 'lower case of',
    year: 'year of',
    quarter: 'quarter of',
    month: 'month of',
    fiscalYear: 'fiscal year of',
    fiscalPeriod: 'fiscal period of',
    not: 'not',
}

function describeLeaf(here: ReportFormulaContext, n: SpecNode, part: ReportPart): string | null {
    switch (part) {
        case 'number':
            return String(n.value)
        case 'text':
            return JSON.stringify(n.value)
        case 'yesno':
            return n.value ? 'yes' : 'no'
        case 'empty':
            return 'empty'
        case 'list': {
            const items = Array.isArray(n.value) ? n.value : []
            return `[${items.map(v => JSON.stringify(v)).join(', ')}]`
        }
        case 'field': {
            const key = n.col ?? n.subtotal ?? ''
            return here.values.find(v => v.key === key)?.label ?? `${key} (missing)`
        }
        case 'path': {
            const info = here.source ? describePath(here.catalog, here.source, n.path ?? []) : null
            return info ? pathLabel(info) : 'a field (missing)'
        }
        case 'record': {
            const record = here.records.find(r => r.group === n.record)
            const info = record ? describePath(here.catalog, record.source, n.path ?? []) : null
            return record && info ? `${record.label} › ${pathLabel(info)}` : `${n.record} (missing)`
        }
        case 'param':
            return `{${here.params.find(p => p.id === n.param)?.label ?? n.param}}`
    }
    return null
}

/** The formula as one line of text, by label: "Amount − Cost", "if Amount > Cost then Amount else Cost". */
export function describeNode(ctx: ReportFormulaContext, node: SpecNode): string {
    const say = (n: SpecNode, here: ReportFormulaContext, nested: boolean): string => {
        const part = partOf(n)
        if (part === 'unknown') return 'a part the editor cannot show'
        const leaf = describeLeaf(here, n, part)
        if (leaf !== null) return leaf
        const args = n.args ?? []
        const sub = (a: SpecNode | undefined) => (a ? say(a, here, true) : '?')
        const flat = (a: SpecNode | undefined) => (a ? say(a, here, false) : '?')
        const list = () => args.map(a => say(a, here, false)).join(', ')
        const wrap = (text: string) => (nested ? `(${text})` : text)
        switch (part) {
            case 'lookup': {
                const inside = contextAt(here, n, [1])
                const source = here.catalog.get(n.source ?? '')?.label ?? `${n.source} (missing)`
                const [condition] = REPORT_TREE.childrenOf(n)
                const matchesAll =
                    !condition || (condition.value === true && onlyKeys(condition, ['value']))
                const where = matchesAll ? '' : ` where ${say(condition, inside, false)}`
                const pick = n.pick === 'max' ? 'largest' : 'smallest'
                return `look up ${say(lookupValueNode(n), inside, false)} in ${source}${where} (${pick})`
            }
            case 'if':
                return wrap(`if ${flat(args[0])} then ${flat(args[1])} else ${flat(args[2])}`)
            case 'round':
                return `round(${flat(args[0])}, ${n.scale})`
            case 'min':
            case 'max':
                return `${part === 'min' ? 'smallest' : 'largest'} of (${list()})`
            case 'and':
            case 'or':
                return wrap(args.map(a => sub(a)).join(` ${part} `))
            case 'isEmpty':
                return wrap(`${sub(args[0])} is empty`)
            case 'between':
                return wrap(`${sub(args[0])} between ${sub(args[1])} and ${sub(args[2])}`)
            case 'inPeriod':
                return wrap(`${sub(args[0])} is in ${sub(args[1])}`)
            case 'in':
                return wrap(`${sub(args[0])} is any of ${sub(args[1])}`)
            case 'case':
                return `first match of (${list()})`
            case 'coalesce':
                return `first not empty of (${list()})`
            case 'concat':
                return args.map(a => sub(a)).join(' & ')
            case 'contains':
            case 'startsWith':
                return wrap(
                    `${sub(args[0])} ${part === 'contains' ? 'contains' : 'starts with'} ${sub(args[1])}`
                )
            case 'addDays':
            case 'addMonths':
                return wrap(
                    `${sub(args[0])} + ${sub(args[1])} ${part === 'addDays' ? 'days' : 'months'}`
                )
            case 'diffDays':
                return `days from ${sub(args[0])} to ${sub(args[1])}`
            case 'percentOfParent':
            case 'percentOfTotal':
                return `${sub(args[0])} as % of ${part === 'percentOfParent' ? 'group' : 'total'}`
        }
        const sign = SIGNS[part]
        if (sign) return wrap(args.map(a => sub(a)).join(sign))
        const word = WORDS[part]
        if (word) return `${word} ${sub(args[0])}`
        return 'a part the editor cannot show'
    }
    return say(node, ctx, false)
}

/** What a formula's type is read from. */
export interface FormulaTypes {
    catalog: Catalog
    source: string | null
    col: (key: string) => FieldType | undefined
    records: readonly FormulaRecord[]
    params: readonly Pick<ParamDef, 'id' | 'type'>[]
}

// params.go paramExpr: a date range is read only by between, as dates.
const PARAM_TYPES: Record<ParamType, FieldType> = {
    date: 'date',
    asOf: 'date',
    dateRange: 'date',
    number: 'number',
    text: 'text',
    choice: 'text',
    amount: 'money',
}

/** The formula's result type, as the engine types it (formula.go operatorExpr). */
export function formulaType(node: SpecNode, types: FormulaTypes): FieldType {
    const t = (n: SpecNode | undefined) => (n ? formulaType(n, types) : 'number')
    const args = node.args ?? []
    // sumType: an amount when any operand is one, else a percent when any is one.
    const sumType = (ns: SpecNode[]): FieldType => {
        const all = ns.map(t)
        if (all.includes('money')) return 'money'
        return all.includes('percent') ? 'percent' : 'number'
    }
    // commonType: the first typed result's type; an amount when any is one.
    const commonType = (ns: SpecNode[]): FieldType => {
        const all = ns.filter(n => partOf(n) !== 'empty').map(t)
        return all.includes('money') ? 'money' : (all[0] ?? 'number')
    }
    switch (partOf(node)) {
        case 'number':
        case 'empty':
            return 'number'
        case 'text':
            return 'text'
        case 'list':
            return Array.isArray(node.value) && typeof node.value[0] === 'number'
                ? 'number'
                : 'text'
        case 'yesno':
            return 'bool'
        case 'field':
            return types.col(node.col ?? node.subtotal ?? '') ?? 'number'
        case 'path': {
            const info = types.source
                ? describePath(types.catalog, types.source, node.path ?? [])
                : null
            return info?.field.type ?? 'text'
        }
        case 'record': {
            const record = types.records.find(r => r.group === node.record)
            const info = record ? describePath(types.catalog, record.source, node.path ?? []) : null
            return info?.field.type ?? 'text'
        }
        case 'param': {
            const param = types.params.find(p => p.id === node.param)
            return param ? PARAM_TYPES[param.type] : 'text'
        }
        case 'lookup':
            return formulaType(lookupValueNode(node), { ...types, source: node.source ?? null })
        case 'add':
        case 'sub':
        case 'min':
        case 'max':
        case 'runningSum':
            return sumType(args)
        case 'mul':
            return args.filter(a => t(a) === 'money').length === 1 ? 'money' : 'number'
        case 'div':
            return t(args[0]) === 'money' && t(args[1]) !== 'money' ? 'money' : 'number'
        // aggregateType: an average stays an amount only in whole cents.
        case 'avg':
            return t(args[0]) === 'money' && node.scale === 0 ? 'money' : 'number'
        case 'round':
        case 'sum':
        case 'smallest':
        case 'largest': {
            const inner = t(args[0])
            return inner === 'bool' ? 'number' : inner
        }
        case 'count':
        case 'countDistinct':
        case 'year':
        case 'quarter':
        case 'month':
        case 'fiscalYear':
        case 'fiscalPeriod':
        case 'diffDays':
            return 'number'
        case 'percentOfParent':
        case 'percentOfTotal':
            return 'percent'
        case 'addDays':
        case 'addMonths':
            return 'date'
        case 'concat':
        case 'upper':
        case 'lower':
            return 'text'
        case 'if':
            return commonType(args.slice(1))
        case 'case': {
            const hasElse = args.length % 2 === 1
            const results = args.filter((_, i) => i % 2 === 1 || (hasElse && i === args.length - 1))
            return commonType(results)
        }
        case 'coalesce':
            return commonType(args)
    }
    return 'bool'
}
