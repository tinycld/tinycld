import type { FormulaPath } from '@tinycld/core/lib/forms/builder/formula'
import { treeAt } from '@tinycld/core/lib/forms/builder/formula-tree'
import { type SpecHop, type SpecNode, specNodeSchema } from '../spec'
import { firstTextField, sourceAtLinks } from './columns'
import { hasScale, onlyKeys } from './formula'
import {
    ARITY,
    type FormulaParam,
    INF,
    isRangeParam,
    isReportPart,
    OP_OF,
    PART_LABELS,
    partOf,
    RELATED_SET,
    REPORT_PARTS,
    REPORT_TREE,
    type ReportFormulaContext,
    type ReportPart,
    SCALED_PARTS,
    TOTAL_SET,
    VALUE_SET,
    WINDOW_SET,
} from './formula-parts'
import { formulaType, typesOf } from './formula-type'

// Editing a report formula (OQ2): which parts fit where, changing a part, adding
// and removing operands, and the scales the engine requires. The part ids and the
// tree are in formula-parts.ts.

export const RATIO_SCALE = 4
const DEFAULT_PLACES = 2

/** The params a node's checks read: their ids and types. */
type ParamTypes = readonly Pick<FormulaParam, 'id' | 'type' | 'multiple'>[]

const hopIs = (hop: SpecHop, key: keyof SpecHop) => {
    const keys = Object.entries(hop).filter(([, value]) => value !== undefined)
    return keys.length === 1 && keys[0]?.[0] === key
}

// A value path the editor writes: links by key, ending at a field or a link
// (bind.go path, pathValue). A hop with any other key (with, mode) stays as it is,
// read-only.
const isFieldPath = (path: readonly SpecHop[]) =>
    path.length > 0 &&
    path.every((hop, i) => hopIs(hop, 'link') || (i === path.length - 1 && hopIs(hop, 'field')))

// An exists link: links only (bind.go path, pathLinks; `with` is refused there).
const isLinkPath = (path: readonly SpecHop[] | undefined) =>
    path !== undefined && path.length > 0 && path.every(hop => hopIs(hop, 'link'))

// A list of texts or of numbers, as `in` compares them (formula.go literal).
const isValueList = (value: unknown) =>
    Array.isArray(value) &&
    (value.every(v => typeof v === 'string') || value.every(v => typeof v === 'number'))

const paramOf = (params: ParamTypes, node: SpecNode | undefined) =>
    node !== undefined && partOf(node) === 'param'
        ? params.find(p => p.id === node.param)
        : undefined

function editable(node: SpecNode, params: ParamTypes, inLookup: boolean): boolean {
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
                REPORT_TREE.childrenOf(node).every(c => editable(c, params, true))
            )
        // Outside a lookup an exists belongs to the filter (formula.go ctxBase.exists).
        case 'exists':
        case 'notExists':
            return (
                inLookup &&
                onlyKeys(node, ['op', 'link', 'where']) &&
                isLinkPath(node.link) &&
                (node.where === undefined || editable(node.where, params, inLookup))
            )
    }
    const [min, max] = ARITY[part] ?? [0, 0]
    const args = node.args ?? []
    const scaleOk = SCALED_PARTS.has(part) ? hasScale(node) : node.scale === undefined
    const period = paramOf(params, args[1])
    const shapeOk = part !== 'inPeriod' || (period !== undefined && isRangeParam(period))
    return (
        onlyKeys(node, ['op', 'args', 'scale']) &&
        args.length >= min &&
        args.length <= max &&
        scaleOk &&
        shapeOk &&
        args.every(a => editable(a, params, inLookup))
    )
}

/**
 * The formula has only shapes this editor writes; anything else stays read-only
 * and is saved unchanged (R17). `params` are the report's params, which tell a
 * date range (the Period of Is in period) from other asked values.
 */
export function isEditableNode(node: SpecNode, params: ParamTypes): boolean {
    return editable(node, params, false)
}

/** What may be read in operand `index` of `node`. */
function childContext(
    here: ReportFormulaContext,
    node: SpecNode,
    index: number
): ReportFormulaContext {
    const part = partOf(node)
    if (part === 'lookup') {
        const source = node.source ?? here.source
        return { ...here, source, totals: false, windows: false, inLookup: true }
    }
    if (RELATED_SET.has(part) && index === 0) {
        const target = here.source
            ? sourceAtLinks(here.catalog, here.source, node.link ?? [])
            : null
        return { ...here, source: target?.key ?? null }
    }
    if (TOTAL_SET.has(part)) return { ...here, totals: false, windows: false }
    if (WINDOW_SET.has(part)) return { ...here, windows: false }
    return here
}

/**
 * What may be read at `path`: inside a lookup its source, no totals, and exists;
 * inside an exists the linked rows; inside a total no total.
 */
export function contextAt(
    ctx: ReportFormulaContext,
    root: SpecNode,
    path: FormulaPath
): ReportFormulaContext {
    let here = ctx
    let node: SpecNode | undefined = root
    for (const index of path) {
        if (!node) break
        here = childContext(here, node, index)
        node = REPORT_TREE.childrenOf(node)[index]
    }
    return here
}

/**
 * The kind of operand a slot takes: a list (the second of Is any of), a period
 * (the second of Is in period), or a single value (everywhere else).
 */
type Slot = 'value' | 'list' | 'period'

function slotAt(root: SpecNode, path: FormulaPath): Slot {
    const index = path[path.length - 1]
    const parent = path.length > 0 ? treeAt(REPORT_TREE, root, path.slice(0, -1)) : undefined
    if (!parent || index !== 1) return 'value'
    const part = partOf(parent)
    if (part === 'in') return 'list'
    if (part === 'inPeriod') return 'period'
    return 'value'
}

const isListParam = (param: Pick<FormulaParam, 'type' | 'multiple'>) =>
    param.type === 'choice' && param.multiple === true

// params.go paramExpr: a date range fits only a period, a multiple choice only a
// list, every other param a single value.
function paramsFor(here: ReportFormulaContext, slot: Slot): FormulaParam[] {
    if (slot === 'period') return here.params.filter(isRangeParam)
    if (slot === 'list') return here.params.filter(isListParam)
    return here.params.filter(p => !isRangeParam(p) && !isListParam(p))
}

function linksOf(here: ReportFormulaContext) {
    return (here.source ? here.catalog.get(here.source)?.links : undefined) ?? []
}

export function partsAt(
    ctx: ReportFormulaContext,
    root: SpecNode,
    path: FormulaPath
): ReportPart[] {
    const here = contextAt(ctx, root, path)
    const slot = slotAt(root, path)
    const hasParams = paramsFor(here, slot).length > 0
    if (slot === 'period') return hasParams ? ['param'] : []
    if (slot === 'list') return hasParams ? ['list', 'param'] : ['list']
    return REPORT_PARTS.filter(part => {
        if (part === 'list') return false
        if (part === 'field') return here.values.length > 0
        if (part === 'path') return here.source !== null
        if (part === 'record') return here.records.length > 0
        if (part === 'param') return hasParams
        if (part === 'inPeriod') return here.params.some(isRangeParam)
        if (part === 'lookup') return here.lookups && here.catalog.size > 0
        if (RELATED_SET.has(part)) return here.inLookup === true && linksOf(here).length > 0
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
    not: { value: true },
    concat: { value: '' },
    coalesce: { value: null },
}

// A date range, a multiple choice param and a list fit only their own slot, so a
// part change never carries one into another part.
function isCarried(here: ReportFormulaContext, node: SpecNode): boolean {
    if (partOf(node) === 'list') return false
    const param = paramOf(here.params, node)
    return param === undefined || (!isRangeParam(param) && !isListParam(param))
}

function seedOf(here: ReportFormulaContext, current: SpecNode): SpecNode[] {
    const part = partOf(current)
    const keepsOne = VALUE_SET.has(part) || part === 'lookup' || RELATED_SET.has(part)
    const seed = keepsOne || part === 'unknown' ? [current] : REPORT_TREE.childrenOf(current)
    return seed.filter(node => isCarried(here, node))
}

// The value a test against a list or a period reads: a between, a period test or a
// list test already holds one first.
function testedValue(here: ReportFormulaContext, current: SpecNode): SpecNode {
    const part = partOf(current)
    const value =
        part === 'between' || part === 'inPeriod' || part === 'in' ? current.args?.[0] : current
    return value && isCarried(here, value) ? value : { value: null }
}

function leafPart(
    here: ReportFormulaContext,
    slot: Slot,
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
            const param = paramsFor(here, slot)[0]
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
        case 'exists':
        case 'notExists': {
            if (RELATED_SET.has(partOf(current))) return { ...current, op: part }
            const link = linksOf(here)[0]
            return link ? { op: part, link: [{ link: link.key }] } : current
        }
    }
    return undefined
}

/**
 * The part at `path` changed to another kind: returns the replacement for that part
 * (not a new root), operands carried over where they fit.
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
    const leaf = leafPart(here, slotAt(root, path), current, part)
    if (leaf) return leaf
    const carried = isCarried(here, current) ? current : { value: null }
    switch (part) {
        case 'if':
            return { op: 'if', args: [{ value: true }, carried, { value: null }] }
        case 'case':
            return { op: 'case', args: [{ value: true }, carried] }
        case 'in':
            return { op: 'in', args: [testedValue(here, current), { value: [] }] }
        case 'inPeriod': {
            const period = here.params.find(isRangeParam)
            if (!period) return current
            return { op: 'between', args: [testedValue(here, current), { param: period.id }] }
        }
    }
    const [min, max] = ARITY[part] ?? [1, 1]
    const filler = FILLER[part] ?? { value: 0 }
    const seed = max === 1 ? [isCarried(here, current) ? current : filler] : seedOf(here, current)
    const padding = Array.from({ length: Math.max(0, min - seed.length) }, () => filler)
    const args = [...seed, ...padding].slice(0, max)
    return settledNode(here, { op: OP_OF[part] ?? part, args })
}

const isAmount = (here: ReportFormulaContext, node: SpecNode | undefined) =>
    node !== undefined && formulaType(node, typesOf(here)) === 'money'

/** The same part in the formula before an edit, and what could be read there. */
interface Before {
    node: SpecNode
    here: ReportFormulaContext
}

// The scale the engine forces for the operands: whole cents for an amount divided
// by a number and for an average of amounts (formula.go div, checkAggregate).
function forcedScale(here: ReportFormulaContext, node: SpecNode): number | undefined {
    const part = partOf(node)
    const [first, second] = node.args ?? []
    if (part === 'div' && isAmount(here, first) && !isAmount(here, second)) return 0
    if (part === 'avg' && isAmount(here, first)) return 0
    return undefined
}

// A scale the engine forces wins. Any other scale is the author's and stays, unless
// it is missing or is still the scale forced before the edit, which no longer
// applies: then a ratio gets RATIO_SCALE places and any other part DEFAULT_PLACES.
// Operands are paired with the formula before the edit by position, so a node that
// moved may meet another's forced scale; only a scale equal to it is reset.
function scaleFor(here: ReportFormulaContext, node: SpecNode, before?: Before): number | undefined {
    const part = partOf(node)
    if (!SCALED_PARTS.has(part)) return undefined
    const forced = forcedScale(here, node)
    if (forced !== undefined) return forced
    const wasForced = before ? forcedScale(before.here, before.node) : undefined
    if (hasScale(node) && node.scale !== wasForced) return node.scale
    return part === 'div' ? RATIO_SCALE : DEFAULT_PLACES
}

function settledNode(here: ReportFormulaContext, node: SpecNode, before?: Before): SpecNode {
    const scale = scaleFor(here, node, before)
    return scale === undefined || scale === node.scale ? node : { ...node, scale }
}

const sameKind = (a: SpecNode | undefined, b: SpecNode) =>
    a !== undefined && partOf(a) === partOf(b)

/**
 * The formula with the scales the engine forces (see scaleFor) after an edit that
 * turned `previous` into `root`. The editor calls it after every edit, since
 * changing an operand can make an amount a number or the other way round; with
 * `previous` it also knows a scale an amount forced that no longer applies.
 */
export function settleScales(
    ctx: ReportFormulaContext,
    root: SpecNode,
    previous?: SpecNode
): SpecNode {
    const walk = (node: SpecNode, here: ReportFormulaContext, before?: Before): SpecNode => {
        const children = REPORT_TREE.childrenOf(node)
        const earlier = before ? REPORT_TREE.childrenOf(before.node) : []
        const next = children.map((child, i) => {
            const was = earlier[i]
            const childBefore =
                before && was && sameKind(was, child)
                    ? { node: was, here: childContext(before.here, before.node, i) }
                    : undefined
            return walk(child, childContext(here, node, i), childBefore)
        })
        const changed = next.some((child, i) => child !== children[i])
        return settledNode(here, changed ? REPORT_TREE.withChildren(node, next) : node, before)
    }
    const before = previous && sameKind(previous, root) ? { node: previous, here: ctx } : undefined
    return walk(root, ctx, before)
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
        case 'exists':
        case 'notExists':
            return index === 0 ? 'Where' : null
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

/** A First match ends in an Otherwise when its operand count is odd (formula.go case). */
export function hasOtherwise(node: SpecNode): boolean {
    return partOf(node) === 'case' && (node.args?.length ?? 0) % 2 === 1
}

const casePairs = (node: SpecNode) => Math.floor((node.args?.length ?? 0) / 2)

/** Gives a First match an Otherwise (empty until set); one that has one is unchanged. */
export function addOtherwise(node: SpecNode): SpecNode {
    if (partOf(node) !== 'case' || hasOtherwise(node)) return node
    return { ...node, args: [...(node.args ?? []), { value: null }] }
}

/** Takes a First match's Otherwise away: no match then gives empty. */
export function removeOtherwise(node: SpecNode): SpecNode {
    if (!hasOtherwise(node)) return node
    return { ...node, args: (node.args ?? []).slice(0, -1) }
}

/** An operand can be removed: a First match keeps one When/Then pair, others their least. */
export function canRemoveNode(node: SpecNode): boolean {
    const part = partOf(node)
    if (part === 'case') return casePairs(node) >= 2
    return isVariadicNode(node) && (node.args?.length ?? 0) > (ARITY[part]?.[0] ?? 0)
}

/** Add part: a case gains a When/Then pair before its Otherwise; others a filler (FILLER). */
export function addOperandNode(node: SpecNode): SpecNode {
    if (!isVariadicNode(node)) return node
    const args = node.args ?? []
    const part = partOf(node)
    if (part === 'case') {
        const pair: SpecNode[] = [{ value: true }, { value: null }]
        const next = hasOtherwise(node)
            ? [...args.slice(0, -1), ...pair, ...args.slice(-1)]
            : [...args, ...pair]
        return { ...node, args: next }
    }
    const filler = part === 'unknown' ? undefined : FILLER[part]
    return { ...node, args: [...args, filler ?? { value: 0 }] }
}

/** Remove part: a case loses its Otherwise, or the When/Then pair the operand belongs to. */
export function removeOperandNode(node: SpecNode, index: number): SpecNode {
    const args = node.args ?? []
    if (hasOtherwise(node) && index === args.length - 1) return removeOtherwise(node)
    if (!canRemoveNode(node)) return node
    if (partOf(node) === 'case') {
        const start = index - (index % 2)
        return { ...node, args: args.filter((_, i) => i !== start && i !== start + 1) }
    }
    return { ...node, args: args.filter((_, i) => i !== index) }
}
