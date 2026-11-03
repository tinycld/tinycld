import { z } from '@tinycld/core/ui/form'
import { isIsoDay } from '../dates'
import { centsFromText, textFromCents } from '../money'
import { type ParamDef, type SpecHop, type SpecNode, specHopSchema, specNodeSchema } from '../spec'
import {
    AGGREGATES,
    type Aggregate,
    type CatalogField,
    type CatalogSource,
    type FieldType,
} from '../types'
import {
    AGGREGATE_LABELS,
    avgScaleFor,
    type Catalog,
    describePath,
    firstTextField,
    pathLabel,
    sourceAtLinks,
} from './columns'
import { hasScale, onlyKeys } from './formula'
import { type FormulaRecord, lookupValueNode } from './formula-parts'

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

/**
 * What a condition compares (R18): a field of the row; on a group level also a
 * total of a field, a column, or a field of the group's record.
 */
export const CONDITION_SUBJECTS = ['field', 'total', 'column', 'record'] as const
export type ConditionSubject = (typeof CONDITION_SUBJECTS)[number]

export type ConditionDraft = {
    kind: 'condition'
    subject: ConditionSubject
    /** The field (subject field or total), or the record's field (subject record). */
    path: SpecHop[]
    /** A total's aggregate (subject total); else ''. */
    aggregate: Aggregate | ''
    /** An average's decimal places; null takes the field type's default (amounts 0). */
    scale: number | null
    /** The column a column subject reads; else ''. */
    col: string
    /** The group whose record a record subject reads; else ''. */
    record: string
    op: ConditionOp
    /** The value, or the low end of between. Money is typed in units (12.50). */
    value: string
    /** The high end of between. */
    high: string
    /** The options of "is any of". */
    values: string[]
    /** A param id when the value is asked when the report runs; else ''. */
    param: string
    /** The param of a range's high end, when both ends are asked; else ''. */
    paramHigh: string
    /** An enclosing group's id when the value is that group's key (a fill); else ''. */
    above: string
}
export type FixedDraft = { kind: 'fixed'; node: SpecNode }
export type GroupDraft = { kind: 'group'; match: 'all' | 'any' | 'none'; items: FilterItem[] }
export type RelatedDraft = { kind: 'related'; has: boolean; link: SpecHop[]; where: GroupDraft }
export type FilterItem = ConditionDraft | RelatedDraft | GroupDraft | FixedDraft

/**
 * A column a group level's condition can read: its label, and its cells as a field
 * (type and options), with the source a relation column points at.
 */
export type LevelColumn = {
    key: string
    label: string
    field: CatalogField
    target?: CatalogSource
}

/** A group level's conditions read its columns and its group records. */
export type LevelScope = {
    columns: readonly LevelColumn[]
    records: readonly FormulaRecord[]
}

const levelColumn = (level: LevelScope | undefined, key: string) =>
    level?.columns.find(c => c.key === key)

/** A group that encloses a level: a fill condition there can read its key ({col}). */
export type AboveGroup = { id: string; col: string; label: string }

/** The groups above level `index`, named by their columns. */
export function aboveOf(
    groups: readonly { id: string; col: string }[],
    index: number,
    columns: readonly { id: string; label?: string }[]
): AboveGroup[] {
    return groups.slice(0, index).map(g => ({
        id: g.id,
        col: g.col,
        label: columns.find(c => c.id === g.col)?.label ?? g.col,
    }))
}

const conditionSchema = z.object({
    kind: z.literal('condition'),
    subject: z.enum(CONDITION_SUBJECTS),
    // A column subject has no path.
    path: z.array(specHopSchema),
    aggregate: z.enum(['', ...AGGREGATES]),
    scale: z.number().int().min(0).max(6).nullable(),
    col: z.string(),
    record: z.string(),
    op: z.enum(CONDITION_OPS),
    value: z.string(),
    high: z.string(),
    values: z.array(z.string()),
    param: z.string(),
    paramHigh: z.string(),
    above: z.string(),
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
    return {
        kind: 'condition',
        subject: 'field',
        path,
        aggregate: '',
        scale: null,
        col: '',
        record: '',
        op: 'eq',
        value: '',
        high: '',
        values: [],
        param: '',
        paramHigh: '',
        above: '',
    }
}

const OPS_BY_TYPE: Record<FieldType, ConditionOp[]> = {
    text: ['eq', 'ne', 'contains', 'startsWith', 'isEmpty', 'notEmpty'],
    relation: ['eq', 'ne', 'in', 'isEmpty', 'notEmpty'],
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

/**
 * The param an asked value becomes, or null when the field cannot be asked (R16).
 * A relation is asked as a record picked from `target`, the source it points at.
 */
export function askParamFor(
    field: CatalogField,
    op: ConditionOp,
    id: string,
    target?: CatalogSource
): ParamDef | null {
    const label = field.label
    if (!needsValue(op)) return null
    if (isDayType(field.type)) {
        if (op === 'between') return { id, type: 'dateRange', label }
        if (op === 'lte') return { id, type: 'asOf', label }
        return op === 'in' ? null : { id, type: 'date', label }
    }
    const ranged = rangedType(field.type)
    if (ranged) return { id, type: ranged, label: op === 'between' ? `${label} from` : label }
    if (field.type === 'relation') return relationParam(id, label, op, target)
    if (field.type === 'text') return { id, type: 'text', label }
    if (field.type === 'select') {
        const options = field.options ?? []
        if (op === 'in') return { id, type: 'choice', label, multiple: true, options }
        return { id, type: 'choice', label, options }
    }
    return null
}

// A record is picked from the target's records, read under the reader's rules (R19);
// without a target only one id can be typed.
function relationParam(
    id: string,
    label: string,
    op: ConditionOp,
    target: CatalogSource | undefined
): ParamDef | null {
    if (!target) return op === 'in' ? null : { id, type: 'text', label }
    const shown = firstTextField(target)?.key ?? 'id'
    const optionsFrom = {
        source: target.key,
        value: [{ field: 'id' }],
        label: [{ field: shown }],
    }
    if (op === 'in') return { id, type: 'choice', label, optionsFrom, multiple: true }
    return { id, type: 'choice', label, optionsFrom }
}

/**
 * A param that lists records, grouped by `group` (a path on the records' source) so
 * the reader picks a whole group with "All <group>" (R30), or ungrouped with null.
 * A param with its own options has no records to group and is returned as it is.
 * The keys keep the spec's order, so a save writes what the engine reads back.
 */
export function withOptionsGroup(param: ParamDef, group: SpecHop[] | null): ParamDef {
    const from = param.optionsFrom
    if (!from) return param
    const optionsFrom = {
        source: from.source,
        value: from.value,
        label: from.label,
        ...(group ? { group } : {}),
        ...(from.where ? { where: from.where } : {}),
    }
    return { ...param, optionsFrom }
}

/** The second param a range of numbers or amounts asks: its high end. */
export function askHighParamFor(field: CatalogField, op: ConditionOp, id: string): ParamDef | null {
    const ranged = rangedType(field.type)
    return op === 'between' && ranged
        ? { id: `${id}To`, type: ranged, label: `${field.label} to` }
        : null
}

// A number asks a number; money asks an amount, kept to its currency (R29).
function rangedType(type: FieldType): 'number' | 'amount' | null {
    if (type === 'number') return 'number'
    return type === 'money' ? 'amount' : null
}

function fieldType(catalog: Catalog, baseKey: string, path: readonly SpecHop[]): FieldType | null {
    return describePath(catalog, baseKey, path)?.field.type ?? null
}

function literal(type: FieldType, text: string): unknown {
    if (isAmountType(type)) return centsFromText(text) ?? 0
    if (type === 'number' || type === 'percent') return Number(text)
    return text
}

// Where a condition tree is read and written: the source its fields are on, and on
// a group level its columns and records (having) or the groups above it (fill).
type Scope = {
    catalog: Catalog
    baseKey: string
    level: LevelScope | undefined
    above: readonly AboveGroup[]
}

// A total that reads one value: one-argument min and max are the smallest and the
// largest of the level's rows.
const AGGREGATE_OPS: ReadonlySet<string> = new Set(AGGREGATES)
const isAggregate = (op: string): op is Aggregate => AGGREGATE_OPS.has(op)
const counts = (aggregate: Aggregate | '') =>
    aggregate === '' || aggregate === 'count' || aggregate === 'countDistinct'

// A row's date is compared by its day (R31); a total or a column is compared as it is.
const usesDay = (subject: ConditionSubject, type: FieldType) =>
    isDayType(type) && (subject === 'field' || subject === 'record')

// The engine averages at a scale it is told: whole cents for amounts, else the
// author's places or the number default (formula.go aggregateNode, checkAggregate).
function avgScale(c: ConditionDraft, field: FieldType | null): number {
    if (field === 'money') return 0
    return c.scale ?? avgScaleFor(field ?? 'number')
}

function subjectNode(c: ConditionDraft, scope: Scope): SpecNode {
    switch (c.subject) {
        case 'total': {
            const node = { op: c.aggregate || 'count', args: [{ path: c.path }] }
            if (c.aggregate !== 'avg') return node
            return { ...node, scale: avgScale(c, fieldType(scope.catalog, scope.baseKey, c.path)) }
        }
        case 'column':
            return { col: c.col }
        case 'record':
            return { record: c.record, path: c.path }
    }
    return { path: c.path }
}

/** The type of what a condition compares, or null when it is no longer offered. */
function subjectType(c: ConditionDraft, scope: Scope): FieldType | null {
    switch (c.subject) {
        case 'column':
            return levelColumn(scope.level, c.col)?.field.type ?? null
        case 'record': {
            const record = scope.level?.records.find(r => r.group === c.record)
            return record ? fieldType(scope.catalog, record.source, c.path) : null
        }
        case 'total': {
            const type = fieldType(scope.catalog, scope.baseKey, c.path)
            return type && counts(c.aggregate) ? 'number' : type
        }
    }
    return fieldType(scope.catalog, scope.baseKey, c.path)
}

function conditionNode(draft: ConditionDraft, type: FieldType, scope: Scope): SpecNode {
    // A value that is the key of a group above (a fill, F21) is not asked.
    const c = draft.above ? { ...draft, param: '', paramHigh: '' } : draft
    const target = subjectNode(c, scope)
    const subject: SpecNode = usesDay(c.subject, type)
        ? { op: 'addDays', args: [target, { value: 0 }] }
        : target
    const group = c.above ? scope.above.find(g => g.id === c.above) : undefined
    const leaf = (text: string): SpecNode => {
        if (group) return { col: group.col }
        return c.param ? { param: c.param } : { value: literal(type, text) }
    }
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
            if (c.param && c.paramHigh)
                return { op: 'between', args: [target, { param: c.param }, { param: c.paramHigh }] }
            // A date range param reads the stored value's day itself (params.go).
            if (c.param) return { op: 'between', args: [target, { param: c.param }] }
            return { op: 'between', args: [subject, leaf(c.value), leaf(c.high)] }
        case 'in':
            return { op: 'in', args: [target, c.param ? { param: c.param } : { value: c.values }] }
        default:
            return { op: c.op, args: [subject, leaf(c.value)] }
    }
}

function groupNode(group: GroupDraft, scope: Scope): SpecNode | undefined {
    const nodes = group.items
        .map(item => itemNode(item, scope))
        .filter((node): node is SpecNode => node !== undefined)
    if (nodes.length === 0) return undefined
    const joined =
        nodes.length === 1 && nodes[0]
            ? nodes[0]
            : { op: group.match === 'all' ? 'and' : 'or', args: nodes }
    return group.match === 'none' ? { op: 'not', args: [joined] } : joined
}

// Related rows' conditions are on the rows the link reaches: no level, no group above.
function relatedScope(scope: Scope, link: readonly SpecHop[]): Scope | null {
    const target = sourceAtLinks(scope.catalog, scope.baseKey, link)
    return target
        ? { catalog: scope.catalog, baseKey: target.key, level: undefined, above: [] }
        : null
}

function itemNode(item: FilterItem, scope: Scope): SpecNode | undefined {
    switch (item.kind) {
        case 'fixed':
            return item.node
        case 'group':
            return groupNode(item, scope)
        case 'related': {
            const inner = relatedScope(scope, item.link)
            const where = inner ? groupNode(item.where, inner) : undefined
            const node: SpecNode = { op: item.has ? 'exists' : 'notExists', link: item.link }
            return where ? { ...node, where } : node
        }
        case 'condition': {
            const type = subjectType(item, scope)
            return type ? conditionNode(item, type, scope) : undefined
        }
    }
}

/**
 * The spec node of a condition tree. `level` is given on a group level (having),
 * `above` in a fill under other groups.
 */
export function filterToNode(
    group: GroupDraft,
    catalog: Catalog,
    baseKey: string,
    level?: LevelScope,
    above: readonly AboveGroup[] = []
): SpecNode | undefined {
    return groupNode(group, { catalog, baseKey, level, above })
}

function plainPath(node: SpecNode | undefined): SpecHop[] | null {
    return node?.path && onlyKeys(node, ['path']) ? node.path : null
}

// The x of addDays(x, 0): a date compared by its day.
function dayInner(node: SpecNode | undefined): SpecNode | null {
    if (node?.op !== 'addDays' || !onlyKeys(node, ['op', 'args'])) return null
    const [subject, zero] = node.args ?? []
    if (node.args?.length !== 2 || !zero || zero.value !== 0 || !onlyKeys(zero, ['value']))
        return null
    return subject ?? null
}

type Leaf = { param: string } | { text: string } | { above: string }

function leafOf(
    node: SpecNode | undefined,
    type: FieldType,
    above: readonly AboveGroup[]
): Leaf | null {
    if (!node) return null
    if (node.param && onlyKeys(node, ['param'])) return { param: node.param }
    if (node.col !== undefined && onlyKeys(node, ['col'])) {
        const group = above.find(g => g.col === node.col)
        return group ? { above: group.id } : null
    }
    if (!onlyKeys(node, ['value'])) return null
    const v = node.value
    if (isAmountType(type))
        return typeof v === 'number' && Number.isSafeInteger(v) ? { text: textFromCents(v) } : null
    if (type === 'number' || type === 'percent')
        return typeof v === 'number' ? { text: String(v) } : null
    return typeof v === 'string' ? { text: v } : null
}

// What a condition node compares: its subject (a field's path, a total, a column or
// a group record's field), read plainly (`raw`) or as the day of a date
// (addDays(x, 0), `day`), and its type.
type Subject = {
    kind: ConditionSubject
    raw: SpecHop[] | null
    day: SpecHop[] | null
    path: SpecHop[]
    type: FieldType
    parts: Partial<ConditionDraft>
}

function condition(
    s: Subject,
    path: SpecHop[],
    op: ConditionOp,
    parts: Partial<ConditionDraft> = {}
): ConditionDraft {
    return { ...emptyCondition(path), ...s.parts, op, ...parts }
}

function fromLeaf(leaf: Leaf): Partial<ConditionDraft> {
    if ('param' in leaf) return { param: leaf.param }
    return 'above' in leaf ? { above: leaf.above } : { value: leaf.text }
}

function paramLeaf(node: SpecNode | undefined): string | null {
    return node?.param && onlyKeys(node, ['param']) ? node.param : null
}

function inCondition(s: Subject, args: SpecNode[]): ConditionDraft | null {
    if (!s.raw || args.length !== 2) return null
    const param = paramLeaf(args[1])
    if (param) return condition(s, s.raw, 'in', { param })
    const list = args[1] && onlyKeys(args[1], ['value']) ? args[1].value : null
    if (!Array.isArray(list) || !list.every(v => typeof v === 'string')) return null
    return condition(s, s.raw, 'in', { values: list })
}

// A date range param, both ends asked (numbers and amounts), or two values.
function betweenCondition(s: Subject, args: SpecNode[], scope: Scope): ConditionDraft | null {
    const param = paramLeaf(args[1])
    if (args.length === 2 && s.raw && param)
        return isDayType(s.type) ? condition(s, s.raw, 'between', { param }) : null
    if (args.length !== 3) return null
    const paramHigh = paramLeaf(args[2])
    if (param && paramHigh)
        return s.raw && rangedType(s.type)
            ? condition(s, s.raw, 'between', { param, paramHigh })
            : null
    if (usesDay(s.kind, s.type) ? !s.day : !s.raw) return null
    const low = leafOf(args[1], s.type, scope.above)
    const high = leafOf(args[2], s.type, scope.above)
    if (low && high && 'text' in low && 'text' in high)
        return condition(s, s.path, 'between', { value: low.text, high: high.text })
    if (low && high && 'above' in low && 'above' in high && low.above === high.above)
        return condition(s, s.path, 'between', { above: low.above })
    return null
}

function boolCondition(s: Subject, args: SpecNode[]): ConditionDraft | null {
    const second = args[1]
    if (!s.raw || args.length !== 2 || s.type !== 'bool' || !second || !onlyKeys(second, ['value']))
        return null
    return typeof second.value === 'boolean'
        ? condition(s, s.raw, second.value ? 'isTrue' : 'isFalse')
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

function compareCondition(
    s: Subject,
    op: string,
    args: SpecNode[],
    scope: Scope
): ConditionDraft | null {
    const known = COMPARE_OPS.find(o => o === op)
    if (!known || args.length !== 2 || (usesDay(s.kind, s.type) ? !s.day : !s.raw)) return null
    const leaf = leafOf(args[1], s.type, scope.above)
    return leaf ? condition(s, s.path, known, fromLeaf(leaf)) : null
}

function subject(
    kind: ConditionSubject,
    path: SpecHop[],
    type: FieldType,
    parts: Partial<ConditionDraft> = {}
): Subject {
    return { kind, raw: path, day: null, path, type, parts: { subject: kind, ...parts } }
}

// A total reads one plain path: sum(x), count(x), min(x), …; an average also says
// its scale.
function totalPath(node: SpecNode): SpecHop[] | null {
    const [arg] = node.args ?? []
    if (!node.op || !isAggregate(node.op)) return null
    const isAvg = node.op === 'avg'
    const keys = isAvg ? ['op', 'args', 'scale'] : ['op', 'args']
    if (!onlyKeys(node, keys) || (isAvg && !hasScale(node))) return null
    return node.args?.length === 1 ? plainPath(arg) : null
}

// An average keeps the author's places; the default reads as null. An average of
// amounts at other places is refused by the engine and stays a fixed part.
function totalParts(
    node: SpecNode,
    op: Aggregate,
    field: FieldType
): Partial<ConditionDraft> | null {
    if (op !== 'avg') return { aggregate: op }
    const scale = node.scale ?? 0
    if (field === 'money' && scale !== 0) return null
    return { aggregate: op, scale: scale === avgScaleFor(field) ? null : scale }
}

// Columns, totals and group records are read only on a group level (R18).
function levelSubject(node: SpecNode, scope: Scope, level: LevelScope): Subject | null {
    if (node.col !== undefined && onlyKeys(node, ['col'])) {
        const type = levelColumn(level, node.col)?.field.type
        return type ? subject('column', [], type, { col: node.col }) : null
    }
    if (node.record !== undefined && onlyKeys(node, ['record', 'path'])) {
        const record = level.records.find(r => r.group === node.record)
        const path = node.path ?? []
        const type = record ? fieldType(scope.catalog, record.source, path) : null
        return type ? subject('record', path, type, { record: node.record }) : null
    }
    const total = totalPath(node)
    if (!total || !node.op || !isAggregate(node.op)) return null
    const field = fieldType(scope.catalog, scope.baseKey, total)
    const parts = field ? totalParts(node, node.op, field) : null
    if (!field || !parts) return null
    return subject('total', total, counts(node.op) ? 'number' : field, parts)
}

function plainSubject(node: SpecNode, scope: Scope): Subject | null {
    const atLevel = scope.level ? levelSubject(node, scope, scope.level) : null
    if (atLevel) return atLevel
    const path = plainPath(node)
    const type = path ? fieldType(scope.catalog, scope.baseKey, path) : null
    return path && type ? subject('field', path, type) : null
}

function subjectOf(node: SpecNode | undefined, scope: Scope): Subject | null {
    if (!node) return null
    const inner = dayInner(node)
    if (!inner) return plainSubject(node, scope)
    const s = plainSubject(inner, scope)
    if (!s || (s.kind !== 'field' && s.kind !== 'record')) return null
    return { ...s, raw: null, day: s.path }
}

function conditionFrom(node: SpecNode, scope: Scope): ConditionDraft | null {
    if (!node.op || !onlyKeys(node, ['op', 'args'])) return null
    const args = node.args ?? []
    const s = subjectOf(args[0], scope)
    if (!s) return null
    if (node.op === 'isEmpty')
        return s.raw && args.length === 1 ? condition(s, s.raw, 'isEmpty') : null
    if (node.op === 'in') return inCondition(s, args)
    if (node.op === 'between') return betweenCondition(s, args, scope)
    const yesNo = node.op === 'eq' ? boolCondition(s, args) : null
    return yesNo ?? compareCondition(s, node.op, args, scope)
}

function isJoin(node: SpecNode): boolean {
    return (
        (node.op === 'and' || node.op === 'or') &&
        onlyKeys(node, ['op', 'args']) &&
        (node.args?.length ?? 0) >= 2
    )
}

function noneFrom(inner: SpecNode, scope: Scope): FilterItem {
    const empty = inner.op === 'isEmpty' ? conditionFrom(inner, scope) : null
    if (empty) return { ...empty, op: 'notEmpty' }
    const items = inner.op === 'or' && isJoin(inner) ? (inner.args ?? []) : [inner]
    return { kind: 'group', match: 'none', items: items.map(a => itemFrom(a, scope)) }
}

function relatedFrom(node: SpecNode, scope: Scope): RelatedDraft | null {
    if (node.op !== 'exists' && node.op !== 'notExists') return null
    if (!node.link || !onlyKeys(node, ['op', 'link', 'where'])) return null
    const inner = relatedScope(scope, node.link)
    if (!inner) return null
    return {
        kind: 'related',
        has: node.op === 'exists',
        link: node.link,
        where: groupFrom(node.where, inner),
    }
}

function itemFrom(node: SpecNode, scope: Scope): FilterItem {
    if (isJoin(node)) {
        return {
            kind: 'group',
            match: node.op === 'and' ? 'all' : 'any',
            items: (node.args ?? []).map(a => itemFrom(a, scope)),
        }
    }
    const inner = node.args?.[0]
    if (node.op === 'not' && onlyKeys(node, ['op', 'args']) && node.args?.length === 1 && inner) {
        return noneFrom(inner, scope)
    }
    return relatedFrom(node, scope) ?? conditionFrom(node, scope) ?? { kind: 'fixed', node }
}

function groupFrom(node: SpecNode | null | undefined, scope: Scope): GroupDraft {
    if (!node) return emptyGroup()
    const item = itemFrom(node, scope)
    return item.kind === 'group' ? item : { kind: 'group', match: 'all', items: [item] }
}

/**
 * The condition tree of a spec node. `level` is given on a group level (having),
 * `above` in a fill under other groups.
 */
export function nodeToFilter(
    node: SpecNode | null | undefined,
    catalog: Catalog,
    baseKey: string,
    level?: LevelScope,
    above: readonly AboveGroup[] = []
): GroupDraft {
    return groupFrom(node, { catalog, baseKey, level, above })
}

/** Every param id a node reads, fixed parts included. */
export function paramsInNode(node: SpecNode | undefined, into: Set<string>): Set<string> {
    if (!node) return into
    if (node.param) into.add(node.param)
    for (const arg of node.args ?? []) paramsInNode(arg, into)
    paramsInNode(node.where, into)
    if (node.op === 'lookup') paramsInNode(lookupValueNode(node), into)
    return into
}

export function paramsInGroup(group: GroupDraft, into: Set<string>): Set<string> {
    for (const item of group.items) {
        // A value that is the key of a group above asks nothing (conditionNode).
        const asks = item.kind === 'condition' && !item.above
        if (asks && item.param) into.add(item.param)
        if (asks && item.paramHigh) into.add(item.paramHigh)
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

function valueProblem(c: ConditionDraft, type: FieldType, scope: Scope): string | null {
    if (!needsValue(c.op)) return null
    // The value is the key of the group above; nothing is asked (conditionNode).
    if (c.above && c.op !== 'in') {
        return scope.above.some(g => g.id === c.above)
            ? null
            : 'the group above is no longer in the report'
    }
    const param = c.above ? '' : c.param
    // Only a date range is one param; a range of numbers asks both ends.
    if (c.op === 'between' && param && !c.paramHigh && !isDayType(type))
        return 'ask both ends of the range'
    if (param) return null
    if (c.op === 'in') return c.values.length > 0 ? null : 'choose at least one'
    const texts = c.op === 'between' ? [c.value, c.high] : [c.value]
    for (const text of texts) {
        const problem = textProblem(text, type)
        if (problem) return problem
    }
    return null
}

const MISSING_SUBJECT: Record<ConditionSubject, string> = {
    field: 'A condition names a field that is no longer offered',
    total: 'A condition names a field that is no longer offered',
    column: 'A condition names a column that is no longer in the report',
    record: 'A condition names a group that no longer links to a record',
}

// A problem names what the person sees: a column by its label, never its id.
function subjectLabel(c: ConditionDraft, scope: Scope): string {
    if (c.subject === 'column') return levelColumn(scope.level, c.col)?.label ?? c.col
    const record =
        c.subject === 'record' ? scope.level?.records.find(r => r.group === c.record) : undefined
    const info = describePath(scope.catalog, record?.source ?? scope.baseKey, c.path)
    if (!info) return ''
    if (c.subject === 'total')
        return `${AGGREGATE_LABELS[c.aggregate || 'count']} of ${pathLabel(info)}`
    return record ? `${record.label} › ${pathLabel(info)}` : info.field.label
}

function problemsIn(group: GroupDraft, scope: Scope): string[] {
    const out: string[] = []
    for (const item of group.items) {
        if (item.kind === 'group') out.push(...problemsIn(item, scope))
        if (item.kind === 'related') {
            const inner = relatedScope(scope, item.link)
            if (inner) out.push(...problemsIn(item.where, inner))
            else out.push('A related-rows condition names a link that is no longer offered')
        }
        if (item.kind !== 'condition') continue
        const type = subjectType(item, scope)
        if (!type) {
            out.push(MISSING_SUBJECT[item.subject])
            continue
        }
        const problem = valueProblem(item, type, scope)
        if (problem) out.push(`${subjectLabel(item, scope)}: ${problem}`)
    }
    return out
}

/** What a person must fix before the conditions can run, one line per condition. */
export function conditionProblems(
    group: GroupDraft,
    catalog: Catalog,
    baseKey: string,
    level?: LevelScope,
    above: readonly AboveGroup[] = []
): string[] {
    return problemsIn(group, { catalog, baseKey, level, above })
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
            return `condition:${item.subject}:${item.col}:${item.record}:${JSON.stringify(item.path)}`
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
