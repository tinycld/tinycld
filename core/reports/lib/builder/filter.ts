import { z } from '@tinycld/core/ui/form'
import { isIsoDay } from '../dates'
import { type ParamDef, type SpecHop, type SpecNode, specHopSchema, specNodeSchema } from '../spec'
import type { CatalogField, FieldType } from '../types'
import { type Catalog, describePath, sourceAtLinks } from './columns'
import { onlyKeys } from './formula'

// The builder's filter: groups of conditions joined by All of / Any of / None of,
// "has / has no related" rows over a link, and fixed parts it cannot edit. Only
// shapes this module writes are read back as editable, so a hand-written or
// shipped filter is never rewritten by a save (R15). A date is compared by its
// day: addDays(field, 0) is the day of a stored date-time (R31).

export const CONDITION_OPS = [
    'eq',
    'ne',
    'lt',
    'lte',
    'gt',
    'gte',
    'between',
    'in',
    'contains',
    'startsWith',
    'isEmpty',
    'notEmpty',
    'isTrue',
    'isFalse',
] as const
export type ConditionOp = (typeof CONDITION_OPS)[number]

export type ConditionDraft = {
    kind: 'condition'
    path: SpecHop[]
    op: ConditionOp
    /** The value, or the low end of between. Money is typed in units (12.50). */
    value: string
    /** The high end of between. */
    high: string
    /** The options of "is any of". */
    values: string[]
    /** A param id when the value is asked when the report runs; else ''. */
    param: string
}
export type FixedDraft = { kind: 'fixed'; node: SpecNode }
export type GroupDraft = { kind: 'group'; match: 'all' | 'any' | 'none'; items: FilterItem[] }
export type RelatedDraft = { kind: 'related'; has: boolean; link: SpecHop[]; where: GroupDraft }
export type FilterItem = ConditionDraft | RelatedDraft | GroupDraft | FixedDraft

const conditionSchema = z.object({
    kind: z.literal('condition'),
    path: z.array(specHopSchema).min(1),
    op: z.enum(CONDITION_OPS),
    value: z.string(),
    high: z.string(),
    values: z.array(z.string()),
    param: z.string(),
})
const fixedSchema = z.object({ kind: z.literal('fixed'), node: specNodeSchema })

export const filterGroupSchema: z.ZodType<GroupDraft, GroupDraft> = z.lazy(() =>
    z.object({
        kind: z.literal('group'),
        match: z.enum(['all', 'any', 'none']),
        items: z.array(filterItemSchema),
    })
)
const relatedSchema: z.ZodType<RelatedDraft, RelatedDraft> = z.lazy(() =>
    z.object({
        kind: z.literal('related'),
        has: z.boolean(),
        link: z.array(specHopSchema).min(1),
        where: filterGroupSchema,
    })
)
const filterItemSchema: z.ZodType<FilterItem, FilterItem> = z.lazy(() =>
    z.union([conditionSchema, relatedSchema, filterGroupSchema, fixedSchema])
)

export function emptyGroup(): GroupDraft {
    return { kind: 'group', match: 'all', items: [] }
}

export function emptyCondition(path: SpecHop[]): ConditionDraft {
    return { kind: 'condition', path, op: 'eq', value: '', high: '', values: [], param: '' }
}

const OPS_BY_TYPE: Record<FieldType, ConditionOp[]> = {
    text: ['eq', 'ne', 'contains', 'startsWith', 'isEmpty', 'notEmpty'],
    relation: ['eq', 'ne', 'isEmpty', 'notEmpty'],
    number: ['eq', 'ne', 'lt', 'lte', 'gt', 'gte', 'between', 'isEmpty', 'notEmpty'],
    money: ['eq', 'ne', 'lt', 'lte', 'gt', 'gte', 'between', 'isEmpty', 'notEmpty'],
    percent: ['eq', 'ne', 'lt', 'lte', 'gt', 'gte', 'between'],
    date: ['eq', 'lt', 'lte', 'gt', 'gte', 'between', 'isEmpty', 'notEmpty'],
    datetime: ['eq', 'lt', 'lte', 'gt', 'gte', 'between', 'isEmpty', 'notEmpty'],
    bool: ['isTrue', 'isFalse'],
    select: ['eq', 'ne', 'in', 'isEmpty', 'notEmpty'],
}

export function opsFor(type: FieldType): ConditionOp[] {
    return OPS_BY_TYPE[type]
}

/** The operators a condition's picker lists: a read-back operator stays listed first. */
export function opsShownFor(type: FieldType, current: ConditionOp): ConditionOp[] {
    const offered = opsFor(type)
    return offered.includes(current) ? offered : [current, ...offered]
}

const isDayType = (type: FieldType) => type === 'date' || type === 'datetime'
const isAmountType = (type: FieldType) => type === 'money'
const isNumberType = (type: FieldType) =>
    type === 'number' || type === 'money' || type === 'percent'

const DAY_LABELS: Partial<Record<ConditionOp, string>> = {
    eq: 'is on',
    lt: 'is before',
    lte: 'is on or before',
    gt: 'is after',
    gte: 'is on or after',
}

const OP_LABELS: Record<ConditionOp, string> = {
    eq: 'is',
    ne: 'is not',
    lt: '<',
    lte: '≤',
    gt: '>',
    gte: '≥',
    between: 'is between',
    in: 'is any of',
    contains: 'contains',
    startsWith: 'starts with',
    isEmpty: 'is empty',
    notEmpty: 'is not empty',
    isTrue: 'is yes',
    isFalse: 'is no',
}

export function opLabel(op: ConditionOp, type: FieldType): string {
    const day = isDayType(type) ? DAY_LABELS[op] : undefined
    if (day) return day
    if (isNumberType(type) && op === 'eq') return '='
    if (isNumberType(type) && op === 'ne') return '≠'
    return OP_LABELS[op]
}

export function needsValue(op: ConditionOp): boolean {
    return !['isEmpty', 'notEmpty', 'isTrue', 'isFalse'].includes(op)
}

export function centsFromText(text: string): number | null {
    const match = /^(-?)(\d+)(?:\.(\d{1,2}))?$/.exec(text.trim())
    if (!match) return null
    const cents = Number(match[2]) * 100 + Number((match[3] ?? '').padEnd(2, '0'))
    return match[1] ? -cents : cents
}

export function textFromCents(cents: number): string {
    const magnitude = Math.abs(cents)
    const text = `${Math.floor(magnitude / 100)}.${String(magnitude % 100).padStart(2, '0')}`
    return cents < 0 ? `-${text}` : text
}

/** The param an asked value becomes, or null when the field cannot be asked (R16). */
export function askParamFor(field: CatalogField, op: ConditionOp, id: string): ParamDef | null {
    const label = field.label
    if (!needsValue(op)) return null
    if (isDayType(field.type)) {
        if (op === 'between') return { id, type: 'dateRange', label }
        if (op === 'lte') return { id, type: 'asOf', label }
        return op === 'in' ? null : { id, type: 'date', label }
    }
    if (field.type === 'number') return op === 'between' ? null : { id, type: 'number', label }
    if (field.type === 'text' || field.type === 'relation') return { id, type: 'text', label }
    if (field.type === 'select') {
        const options = field.options ?? []
        if (op === 'in') return { id, type: 'choice', label, multiple: true, options }
        return { id, type: 'choice', label, options }
    }
    return null
}

function fieldType(catalog: Catalog, baseKey: string, path: readonly SpecHop[]): FieldType | null {
    return describePath(catalog, baseKey, path)?.field.type ?? null
}

function literal(type: FieldType, text: string): unknown {
    if (isAmountType(type)) return centsFromText(text) ?? 0
    if (type === 'number' || type === 'percent') return Number(text)
    return text
}

function conditionNode(c: ConditionDraft, type: FieldType): SpecNode {
    const target: SpecNode = { path: c.path }
    const subject: SpecNode = isDayType(type)
        ? { op: 'addDays', args: [target, { value: 0 }] }
        : target
    const leaf = (text: string): SpecNode =>
        c.param ? { param: c.param } : { value: literal(type, text) }
    switch (c.op) {
        case 'isEmpty':
            return { op: 'isEmpty', args: [target] }
        case 'notEmpty':
            return { op: 'not', args: [{ op: 'isEmpty', args: [target] }] }
        case 'isTrue':
            return { op: 'eq', args: [target, { value: true }] }
        case 'isFalse':
            return { op: 'eq', args: [target, { value: false }] }
        case 'between':
            // A date range param reads the stored value's day itself (params.go).
            if (c.param) return { op: 'between', args: [target, { param: c.param }] }
            return { op: 'between', args: [subject, leaf(c.value), leaf(c.high)] }
        case 'in':
            return { op: 'in', args: [target, c.param ? { param: c.param } : { value: c.values }] }
        default:
            return { op: c.op, args: [subject, leaf(c.value)] }
    }
}

function groupNode(group: GroupDraft, catalog: Catalog, baseKey: string): SpecNode | undefined {
    const nodes = group.items
        .map(item => itemNode(item, catalog, baseKey))
        .filter((node): node is SpecNode => node !== undefined)
    if (nodes.length === 0) return undefined
    const joined =
        nodes.length === 1 && nodes[0]
            ? nodes[0]
            : { op: group.match === 'all' ? 'and' : 'or', args: nodes }
    return group.match === 'none' ? { op: 'not', args: [joined] } : joined
}

function itemNode(item: FilterItem, catalog: Catalog, baseKey: string): SpecNode | undefined {
    switch (item.kind) {
        case 'fixed':
            return item.node
        case 'group':
            return groupNode(item, catalog, baseKey)
        case 'related': {
            const target = sourceAtLinks(catalog, baseKey, item.link)
            const where = target ? groupNode(item.where, catalog, target.key) : undefined
            const node: SpecNode = { op: item.has ? 'exists' : 'notExists', link: item.link }
            return where ? { ...node, where } : node
        }
        case 'condition': {
            const type = fieldType(catalog, baseKey, item.path)
            return type ? conditionNode(item, type) : undefined
        }
    }
}

export function filterToNode(
    group: GroupDraft,
    catalog: Catalog,
    baseKey: string
): SpecNode | undefined {
    return groupNode(group, catalog, baseKey)
}

function plainPath(node: SpecNode | undefined): SpecHop[] | null {
    return node?.path && onlyKeys(node, ['path']) ? node.path : null
}

function dayPath(node: SpecNode | undefined): SpecHop[] | null {
    if (node?.op !== 'addDays' || !onlyKeys(node, ['op', 'args'])) return null
    const [subject, zero] = node.args ?? []
    if (node.args?.length !== 2 || !zero || zero.value !== 0 || !onlyKeys(zero, ['value']))
        return null
    return plainPath(subject)
}

type Leaf = { param: string } | { text: string }

function leafOf(node: SpecNode | undefined, type: FieldType): Leaf | null {
    if (!node) return null
    if (node.param && onlyKeys(node, ['param'])) return { param: node.param }
    if (!onlyKeys(node, ['value'])) return null
    const v = node.value
    if (isAmountType(type))
        return typeof v === 'number' && Number.isSafeInteger(v) ? { text: textFromCents(v) } : null
    if (type === 'number' || type === 'percent')
        return typeof v === 'number' ? { text: String(v) } : null
    return typeof v === 'string' ? { text: v } : null
}

function condition(
    path: SpecHop[],
    op: ConditionOp,
    parts: Partial<ConditionDraft> = {}
): ConditionDraft {
    return { ...emptyCondition(path), op, ...parts }
}

function fromLeaf(leaf: Leaf): Partial<ConditionDraft> {
    return 'param' in leaf ? { param: leaf.param } : { value: leaf.text }
}

// What a condition node compares: the field's path, as written plainly or as the
// day of a date (addDays(path, 0)), and the field's type.
type Subject = { raw: SpecHop[] | null; day: SpecHop[] | null; path: SpecHop[]; type: FieldType }

function paramLeaf(node: SpecNode | undefined): string | null {
    return node?.param && onlyKeys(node, ['param']) ? node.param : null
}

function inCondition(s: Subject, args: SpecNode[]): ConditionDraft | null {
    if (!s.raw || args.length !== 2) return null
    const param = paramLeaf(args[1])
    if (param) return condition(s.raw, 'in', { param })
    const list = args[1] && onlyKeys(args[1], ['value']) ? args[1].value : null
    if (!Array.isArray(list) || !list.every(v => typeof v === 'string')) return null
    return condition(s.raw, 'in', { values: list })
}

function betweenCondition(s: Subject, args: SpecNode[]): ConditionDraft | null {
    const wantsDay = isDayType(s.type)
    const param = paramLeaf(args[1])
    if (args.length === 2 && s.raw && param)
        return wantsDay ? condition(s.raw, 'between', { param }) : null
    if (args.length !== 3 || (wantsDay ? !s.day : !s.raw)) return null
    const low = leafOf(args[1], s.type)
    const high = leafOf(args[2], s.type)
    if (!low || !high || 'param' in low || 'param' in high) return null
    return condition(s.path, 'between', { value: low.text, high: high.text })
}

function boolCondition(s: Subject, args: SpecNode[]): ConditionDraft | null {
    const second = args[1]
    if (!s.raw || args.length !== 2 || s.type !== 'bool' || !second || !onlyKeys(second, ['value']))
        return null
    return typeof second.value === 'boolean'
        ? condition(s.raw, second.value ? 'isTrue' : 'isFalse')
        : null
}

const COMPARE_OPS: readonly ConditionOp[] = [
    'eq',
    'ne',
    'lt',
    'lte',
    'gt',
    'gte',
    'contains',
    'startsWith',
]

function compareCondition(s: Subject, op: string, args: SpecNode[]): ConditionDraft | null {
    const known = COMPARE_OPS.find(o => o === op)
    if (!known || args.length !== 2 || (isDayType(s.type) ? !s.day : !s.raw)) return null
    const leaf = leafOf(args[1], s.type)
    return leaf ? condition(s.path, known, fromLeaf(leaf)) : null
}

function subjectOf(node: SpecNode | undefined, catalog: Catalog, baseKey: string): Subject | null {
    const raw = plainPath(node)
    const day = dayPath(node)
    const path = raw ?? day
    const type = path ? fieldType(catalog, baseKey, path) : null
    return path && type ? { raw, day, path, type } : null
}

function conditionFrom(node: SpecNode, catalog: Catalog, baseKey: string): ConditionDraft | null {
    if (!node.op || !onlyKeys(node, ['op', 'args'])) return null
    const args = node.args ?? []
    const s = subjectOf(args[0], catalog, baseKey)
    if (!s) return null
    if (node.op === 'isEmpty')
        return s.raw && args.length === 1 ? condition(s.raw, 'isEmpty') : null
    if (node.op === 'in') return inCondition(s, args)
    if (node.op === 'between') return betweenCondition(s, args)
    const yesNo = node.op === 'eq' ? boolCondition(s, args) : null
    return yesNo ?? compareCondition(s, node.op, args)
}

function isJoin(node: SpecNode): boolean {
    return (
        (node.op === 'and' || node.op === 'or') &&
        onlyKeys(node, ['op', 'args']) &&
        (node.args?.length ?? 0) >= 2
    )
}

function noneFrom(inner: SpecNode, catalog: Catalog, baseKey: string): FilterItem {
    const empty = inner.op === 'isEmpty' ? conditionFrom(inner, catalog, baseKey) : null
    if (empty) return { ...empty, op: 'notEmpty' }
    const items = inner.op === 'or' && isJoin(inner) ? (inner.args ?? []) : [inner]
    return { kind: 'group', match: 'none', items: items.map(a => itemFrom(a, catalog, baseKey)) }
}

function relatedFrom(node: SpecNode, catalog: Catalog, baseKey: string): RelatedDraft | null {
    if (node.op !== 'exists' && node.op !== 'notExists') return null
    if (!node.link || !onlyKeys(node, ['op', 'link', 'where'])) return null
    const target = sourceAtLinks(catalog, baseKey, node.link)
    if (!target) return null
    return {
        kind: 'related',
        has: node.op === 'exists',
        link: node.link,
        where: nodeToFilter(node.where, catalog, target.key),
    }
}

function itemFrom(node: SpecNode, catalog: Catalog, baseKey: string): FilterItem {
    if (isJoin(node)) {
        return {
            kind: 'group',
            match: node.op === 'and' ? 'all' : 'any',
            items: (node.args ?? []).map(a => itemFrom(a, catalog, baseKey)),
        }
    }
    const inner = node.args?.[0]
    if (node.op === 'not' && onlyKeys(node, ['op', 'args']) && node.args?.length === 1 && inner) {
        return noneFrom(inner, catalog, baseKey)
    }
    return (
        relatedFrom(node, catalog, baseKey) ??
        conditionFrom(node, catalog, baseKey) ?? { kind: 'fixed', node }
    )
}

export function nodeToFilter(
    node: SpecNode | null | undefined,
    catalog: Catalog,
    baseKey: string
): GroupDraft {
    if (!node) return emptyGroup()
    const item = itemFrom(node, catalog, baseKey)
    return item.kind === 'group' ? item : { kind: 'group', match: 'all', items: [item] }
}

/** Every param id a node reads, fixed parts included. */
export function paramsInNode(node: SpecNode | undefined, into: Set<string>): Set<string> {
    if (!node) return into
    if (node.param) into.add(node.param)
    for (const arg of node.args ?? []) paramsInNode(arg, into)
    paramsInNode(node.where, into)
    return into
}

export function paramsInGroup(group: GroupDraft, into: Set<string>): Set<string> {
    for (const item of group.items) {
        if (item.kind === 'condition' && item.param) into.add(item.param)
        if (item.kind === 'fixed') paramsInNode(item.node, into)
        if (item.kind === 'group') paramsInGroup(item, into)
        if (item.kind === 'related') paramsInGroup(item.where, into)
    }
    return into
}

function textProblem(text: string, type: FieldType): string | null {
    if (text.trim() === '') return 'enter a value'
    if (isAmountType(type) && centsFromText(text) === null) return 'enter an amount such as 12.50'
    if ((type === 'number' || type === 'percent') && !Number.isFinite(Number(text)))
        return 'enter a number'
    if (isDayType(type) && !isIsoDay(text)) return 'enter a date as YYYY-MM-DD'
    return null
}

function valueProblem(c: ConditionDraft, field: CatalogField): string | null {
    if (!needsValue(c.op) || c.param) return null
    if (c.op === 'in') return c.values.length > 0 ? null : 'choose at least one'
    const texts = c.op === 'between' ? [c.value, c.high] : [c.value]
    for (const text of texts) {
        const problem = textProblem(text, field.type)
        if (problem) return problem
    }
    return null
}

/** What a person must fix before the filter can run, one line per condition. */
export function conditionProblems(group: GroupDraft, catalog: Catalog, baseKey: string): string[] {
    const out: string[] = []
    for (const item of group.items) {
        if (item.kind === 'group') out.push(...conditionProblems(item, catalog, baseKey))
        if (item.kind === 'related') {
            const target = sourceAtLinks(catalog, baseKey, item.link)
            if (target) out.push(...conditionProblems(item.where, catalog, target.key))
            else out.push('A related-rows condition names a link that is no longer offered')
        }
        if (item.kind !== 'condition') continue
        const info = describePath(catalog, baseKey, item.path)
        if (!info) {
            out.push('A condition names a field that is no longer offered')
            continue
        }
        const problem = valueProblem(item, info.field)
        if (problem) out.push(`${info.field.label}: ${problem}`)
    }
    return out
}

/** An item's place in the tree: indexes from the root; a related item's children are its conditions. */
export type FilterPath = readonly number[]

function childItems(item: FilterItem): FilterItem[] | null {
    if (item.kind === 'group') return item.items
    if (item.kind === 'related') return item.where.items
    return null
}

function withChildItems(item: FilterItem, items: FilterItem[]): FilterItem {
    if (item.kind === 'group') return { ...item, items }
    if (item.kind === 'related') return { ...item, where: { ...item.where, items } }
    return item
}

// Replaces the item at `path` with fn's answer; null removes it. A path that names
// no item leaves the tree as it is.
function mapAt(
    item: FilterItem,
    path: FilterPath,
    fn: (item: FilterItem) => FilterItem | null
): FilterItem | null {
    if (path.length === 0) return fn(item)
    const [head, ...rest] = path
    const items = childItems(item)
    const child = head === undefined ? undefined : items?.[head]
    if (!items || child === undefined) return item
    const next = mapAt(child, rest, fn)
    const changed =
        next === null
            ? items.filter((_, i) => i !== head)
            : items.map((c, i) => (i === head ? next : c))
    return withChildItems(item, changed)
}

function asGroup(item: FilterItem | null, fallback: GroupDraft): GroupDraft {
    return item?.kind === 'group' ? item : fallback
}

export function updateFilterItem(
    root: GroupDraft,
    path: FilterPath,
    fn: (item: FilterItem) => FilterItem
): GroupDraft {
    return asGroup(mapAt(root, path, fn), root)
}

export function removeFilterItem(root: GroupDraft, path: FilterPath): GroupDraft {
    if (path.length === 0) return root
    return asGroup(
        mapAt(root, path, () => null),
        root
    )
}

/** Adds an item to the group, or the related item's conditions, at `path`. */
export function addFilterItem(root: GroupDraft, path: FilterPath, added: FilterItem): GroupDraft {
    return asGroup(
        mapAt(root, path, item => withChildItems(item, [...(childItems(item) ?? []), added])),
        root
    )
}

export function setMatchAt(
    root: GroupDraft,
    path: FilterPath,
    match: GroupDraft['match']
): GroupDraft {
    return asGroup(
        mapAt(root, path, item => {
            if (item.kind === 'group') return { ...item, match }
            if (item.kind === 'related') return { ...item, where: { ...item.where, match } }
            return item
        }),
        root
    )
}

function itemSignature(item: FilterItem): string {
    switch (item.kind) {
        case 'condition':
            return `condition:${JSON.stringify(item.path)}`
        case 'related':
            return `related:${JSON.stringify(item.link)}`
        case 'group':
            return 'group'
        case 'fixed':
            return `fixed:${JSON.stringify(item.node)}`
    }
}

/**
 * A key per item for the editor's rows: what the item filters on, and how many
 * earlier siblings filter on the same. Typing a value keeps a row's key, and
 * removing a row does not hand its key to the next one on another field.
 */
export function filterItemKeys(items: readonly FilterItem[]): string[] {
    const seen = new Map<string, number>()
    return items.map(item => {
        const signature = itemSignature(item)
        const n = seen.get(signature) ?? 0
        seen.set(signature, n + 1)
        return `${signature}#${n}`
    })
}

/**
 * The params after a filter change: those the new filter asks for or something
 * else reads (`readElsewhere`, e.g. a fixed column formula), plus a newly asked
 * one. A removed condition, group or related row takes its asked values with it.
 */
export function filterParams(
    params: readonly ParamDef[],
    filter: GroupDraft,
    readElsewhere: ReadonlySet<string>,
    added?: ParamDef
): ParamDef[] {
    const asked = paramsInGroup(filter, new Set(readElsewhere))
    const kept = params.filter(p => asked.has(p.id))
    return added ? [...kept, added] : kept
}
