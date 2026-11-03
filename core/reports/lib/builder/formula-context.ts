import type { SpecHop, SpecNode } from '../spec'
import type { CatalogSource, FieldType } from '../types'
import { aggregateResultType, type Catalog, describePath } from './columns'
import type { BuilderDraft, ColumnDraft } from './draft'
import {
    type FormulaParam,
    type FormulaRecord,
    lookupValueNode,
    type ReportFormulaContext,
} from './formula-parts'
import { type FormulaTypes, formulaType } from './formula-type'

// What a report formula may read where the builder edits it (OQ2): the place
// decides the leaf ({col} or {subtotal}), the values offered, and whether totals,
// running values and lookups fit there (formula.go, layout.go, lookup.go). The
// type of each column's cells is here too, since a formula reads them by id.

/**
 * Where a formula is edited: a column (`selfKey` is its own id), a layout computed
 * row (`index` is its place in the layout), a group level's label (`index` is the
 * level: it reads only what the level's rows have), a sort formula (a level with no
 * index: a sort reads anything but a running value, and skips a level where it has
 * no value) or a detail row's label.
 */
export type FormulaPlace =
    | { kind: 'column'; selfKey: string }
    | { kind: 'layout'; index: number }
    | { kind: 'level'; index?: number }
    | { kind: 'detail' }

/**
 * What a column's type is read from: the columns, and the groups and params a
 * formula reading a record or a param is typed by. Every caller passes all four,
 * so a column has one type in every section and in the saved spec.
 */
export type ColumnsOf = Pick<BuilderDraft, 'source' | 'columns' | 'params'> & {
    groups: readonly GroupKey[]
}

/**
 * What a group level is keyed by: enough to tell which levels carry a record, and
 * (with no bucket) whose column has its value on the level's rows.
 */
export type GroupKey = { id: string; col: string; bucket?: string }

const WINDOW_OPS: ReadonlySet<string> = new Set(['runningSum', 'percentOfParent', 'percentOfTotal'])

// Every node a formula reads: its operands, a where, and a lookup's value.
function childNodes(node: SpecNode): SpecNode[] {
    const out = [...(node.args ?? [])]
    if (node.where) out.push(node.where)
    if (node.op === 'lookup') out.push(lookupValueNode(node))
    return out
}

/** Whether a formula reads column `col` anywhere. */
export function readsColumn(node: SpecNode | undefined, col: string): boolean {
    if (!node) return false
    return node.col === col || childNodes(node).some(n => readsColumn(n, col))
}

/**
 * The columns whose cells are a running value or a percent, or read one (formula.go
 * window): such a cell is not added up, sorted on, or read on a group row.
 */
export function windowColumns(columns: readonly ColumnDraft[]): ReadonlySet<string> {
    const byId = new Map(columns.map(c => [c.id, c]))
    const memo = new Map<string, boolean>()
    const seen = new Set<string>()
    const isWindow = (id: string): boolean => {
        const known = memo.get(id)
        if (known !== undefined) return known
        const column = byId.get(id)
        if (column?.kind !== 'formula' || seen.has(id)) return false
        seen.add(id)
        const result = readsWindow(column.formula.node)
        seen.delete(id)
        memo.set(id, result)
        return result
    }
    const readsWindow = (node: SpecNode): boolean =>
        (node.op !== undefined && WINDOW_OPS.has(node.op)) ||
        (node.col !== undefined && isWindow(node.col)) ||
        childNodes(node).some(readsWindow)
    return new Set(columns.filter(c => isWindow(c.id)).map(c => c.id))
}

// A total reads many rows: sum, count, average, and min or max of one value
// (levels.go aggregateOp).
function isAggregateNode(node: SpecNode): boolean {
    if (node.op === 'sum' || node.op === 'count' || node.op === 'countDistinct') return true
    if (node.op === 'avg') return true
    return (node.op === 'min' || node.op === 'max') && node.args?.length === 1
}

type LevelDraft = Pick<BuilderDraft, 'source' | 'columns'> & { groups: readonly GroupKey[] }

/**
 * Whether a column has no one value on the rows of group level `index` (levels.go
 * readsRowValue): a field outside a total, a record of a group inside the level,
 * or a column that reads one.
 */
function rowValueReader(draft: LevelDraft, catalog: Catalog, index: number) {
    const byId = new Map(draft.columns.map(c => [c.id, c]))
    const memo = new Map<string, boolean>()
    const seen = new Set<string>()
    // A group key with no bucket at or above the level is the column's value there.
    const isKeyHere = (id: string) =>
        draft.groups.slice(0, index + 1).some(g => g.col === id && !g.bucket)
    const columnReads = (id: string): boolean => {
        const column = byId.get(id)
        // An unknown column is refused by its own check, not read here.
        if (!column) return false
        if (column.aggregate !== '' || column.total === 'rows' || isKeyHere(id)) return false
        // A path reads a row's value, unless it totals a to-many link per row.
        if (column.kind === 'path')
            return describePath(catalog, draft.source, column.path)?.mode !== 'aggregate'
        if (draft.groups.some(g => g.col === id)) return true
        const known = memo.get(id)
        if (known !== undefined) return known
        seen.add(id)
        const result = reads(column.formula.node, false)
        seen.delete(id)
        memo.set(id, result)
        return result
    }
    // A lookup's paths read the looked-up source; only the report's values it reads count.
    const lookupReads = (node: SpecNode, inAggregate: boolean): boolean => {
        const walk = (n: SpecNode): boolean => {
            if (n.col !== undefined || n.record !== undefined) return reads(n, inAggregate)
            if (n.path) return false
            return childNodes(n).some(walk)
        }
        return walk(node)
    }
    const reads = (node: SpecNode, inAggregate: boolean): boolean => {
        if (node.op === 'lookup') return lookupReads(node, inAggregate)
        if (node.record !== undefined) {
            const at = draft.groups.findIndex(g => g.id === node.record)
            return !inAggregate && at > index
        }
        if (node.path) return !inAggregate
        if (node.col !== undefined && !inAggregate && !seen.has(node.col))
            return columnReads(node.col)
        const inner = inAggregate || isAggregateNode(node)
        return (node.args ?? []).some(a => reads(a, inner))
    }
    return columnReads
}

/**
 * The columns that have one value on the rows of group level `index` and are not a
 * running value or a percent: what the level's label and having may read (levels.go
 * labelSQL, own).
 */
export function levelValueColumns(
    draft: LevelDraft,
    catalog: Catalog,
    index: number
): ReadonlySet<string> {
    const windows = windowColumns(draft.columns)
    const columnReads = rowValueReader(draft, catalog, index)
    return new Set(
        draft.columns.filter(c => !windows.has(c.id) && !columnReads(c.id)).map(c => c.id)
    )
}

/**
 * The type of each column's cells, by id, for formula operands, division and
 * average scales, pivot cells and layout keys. A formula column is typed as the
 * engine types it (formula-type.ts) from what it reads.
 */
export function columnTypes(draft: ColumnsOf, catalog: Catalog): ReadonlyMap<string, FieldType> {
    const byId = new Map(draft.columns.map(column => [column.id, column]))
    const types = new Map<string, FieldType>()
    const visiting = new Set<string>()
    const typeOf = (column: ColumnDraft): FieldType => {
        const known = types.get(column.id)
        if (known) return known
        // The engine refuses a formula that reads itself; read it as a number.
        if (visiting.has(column.id)) return 'number'
        visiting.add(column.id)
        const type = cellType(column)
        visiting.delete(column.id)
        types.set(column.id, type)
        return type
    }
    const formulaTypes: FormulaTypes = {
        catalog,
        source: draft.source,
        col: key => {
            const read = byId.get(key)
            return read ? typeOf(read) : undefined
        },
        records: recordGroups(draft, catalog),
        params: draft.params,
    }
    const cellType = (column: ColumnDraft): FieldType => {
        if (column.kind === 'path') {
            const info = describePath(catalog, draft.source, column.path)
            return info ? aggregateResultType(column.aggregate, info.field.type) : 'text'
        }
        return aggregateResultType(column.aggregate, formulaType(column.formula.node, formulaTypes))
    }
    for (const column of draft.columns) typeOf(column)
    return types
}

/** Cells of this type are numbers: a formula column giving one is shown as a number or a percent. */
export const isNumericType = (type: FieldType) =>
    type === 'number' || type === 'money' || type === 'percent'

/** The columns a formula, a group or a pivot cell can name. */
export function columnChoices(draft: ColumnsOf, catalog: Catalog) {
    const types = columnTypes(draft, catalog)
    return draft.columns.map(column => {
        const type = types.get(column.id) ?? 'text'
        const info =
            column.kind === 'path' ? describePath(catalog, draft.source, column.path) : null
        return {
            key: column.id,
            label: column.label,
            type,
            isNumeric: isNumericType(type),
            hasAggregate: column.aggregate !== '',
            isDate: type === 'date' || type === 'datetime',
            isNested: info?.mode === 'nest',
            options: info?.field.options ?? [],
        }
    })
}

export type ColumnChoice = ReturnType<typeof columnChoices>[number]

/**
 * The columns whose total may add the rows shown: numbers and amounts, not child
 * rows, running values or percents (levels.go detailCell).
 */
export function rowsTotalColumns(
    choices: readonly ColumnChoice[],
    columns: readonly ColumnDraft[]
): ReadonlySet<string> {
    const windows = windowColumns(columns)
    return new Set(
        choices.filter(c => c.isNumeric && !c.isNested && !windows.has(c.key)).map(c => c.key)
    )
}

/** What a column's record is read from: the columns, and the groups a record leaf names. */
export type TargetDraft = Pick<BuilderDraft, 'source' | 'columns'> & {
    groups: readonly GroupKey[]
}

// A value path's record, as the compiler's pathTarget finds it: the end of to-one
// links, or the source whose id it reads. A to-many hop makes a total, which points
// nowhere.
function pathTarget(
    catalog: Catalog,
    sourceKey: string | undefined,
    path: readonly SpecHop[]
): CatalogSource | undefined {
    let source = sourceKey === undefined ? undefined : catalog.get(sourceKey)
    for (const hop of path) {
        if (!source) return undefined
        if (hop.link === undefined) return hop.field === 'id' ? source : undefined
        const link = source.links.find(l => l.key === hop.link)
        if (link?.cardinality !== 'toOne') return undefined
        source = catalog.get(link.target)
    }
    return source
}

// case's results: the second of each condition and result pair, then an else.
function caseResults(args: readonly SpecNode[]): SpecNode[] {
    const out: SpecNode[] = []
    for (let i = 1; i < args.length; i += 2) {
        const result = args[i]
        if (result) out.push(result)
    }
    const otherwise = args[args.length - 1]
    return args.length % 2 === 1 && otherwise ? [...out, otherwise] : out
}

/**
 * Reads the record a column's cells point at, as the compiler finds it (alias.go
 * targetOf, nodeTarget): a path that ends at a to-one link or at id, a column a
 * formula names, a group's record, a lookup's value, or the first result of an if,
 * a case or a coalesce that points at one (the engine refuses results that point at
 * different sources).
 */
function targetReader(draft: TargetDraft, catalog: Catalog) {
    const byId = new Map(draft.columns.map(c => [c.id, c]))
    // A formula that reads itself through other columns or groups points nowhere.
    const visiting = new Set<string>()
    const ofColumn = (column: ColumnDraft): CatalogSource | undefined => {
        if (visiting.has(column.id)) return undefined
        visiting.add(column.id)
        const target =
            column.kind === 'path'
                ? pathTarget(catalog, draft.source, column.path)
                : ofNode(draft.source, column.formula.node)
        visiting.delete(column.id)
        return target
    }
    // A bucketed date has no record (rows.go reads a target only unbucketed); with
    // two groups of one id the first counts, as in the compiler.
    const ofGroup = (id: string): CatalogSource | undefined => {
        const group = draft.groups.find(g => g.id === id)
        const column = byId.get(group?.col ?? '')
        return group && !group.bucket && column ? ofColumn(column) : undefined
    }
    const first = (sourceKey: string, nodes: readonly SpecNode[]) => {
        for (const node of nodes) {
            const target = ofNode(sourceKey, node)
            if (target) return target
        }
        return undefined
    }
    const ofNode = (sourceKey: string, node: SpecNode): CatalogSource | undefined => {
        if (node.record !== undefined)
            return pathTarget(catalog, ofGroup(node.record)?.key, node.path ?? [])
        if (node.path) return pathTarget(catalog, sourceKey, node.path)
        if (node.col !== undefined) {
            const column = byId.get(node.col)
            return column ? ofColumn(column) : undefined
        }
        const args = node.args ?? []
        switch (node.op) {
            case 'lookup':
                return node.source ? ofNode(node.source, lookupValueNode(node)) : undefined
            case 'if':
                // The first argument is the condition, which points nowhere.
                return first(sourceKey, args.slice(1))
            case 'case':
                return first(sourceKey, caseResults(args))
            case 'coalesce':
                return first(sourceKey, args)
        }
        return undefined
    }
    return { ofColumn }
}

/** The source column `id`'s cells point at (a record), or undefined. */
export function columnTarget(
    draft: TargetDraft,
    catalog: Catalog,
    id: string
): CatalogSource | undefined {
    const column = draft.columns.find(c => c.id === id)
    return column ? targetReader(draft, catalog).ofColumn(column) : undefined
}

/**
 * The source a link column's cells point at, when they are not totalled (R31): its
 * cells show the record's label field, which "Show as" picks.
 */
export function linkTarget(
    draft: TargetDraft,
    catalog: Catalog,
    column: ColumnDraft
): CatalogSource | undefined {
    if (column.aggregate !== '') return undefined
    return targetReader(draft, catalog).ofColumn(column)
}

/**
 * The columns a sort key may name: not child rows, which keep their own order
 * (bind.go sort), nor a running value or a percent (run.go notASortKey).
 */
export function sortColumns(
    choices: readonly ColumnChoice[],
    columns: readonly ColumnDraft[]
): ColumnChoice[] {
    const windows = windowColumns(columns)
    return choices.filter(c => !c.isNested && !windows.has(c.key))
}

/** The group levels whose column points at one record: they carry that record. */
export function recordGroups(draft: TargetDraft, catalog: Catalog): FormulaRecord[] {
    const read = targetReader(draft, catalog)
    return draft.groups.flatMap(group => {
        const column = draft.columns.find(c => c.id === group.col)
        const target = column && !group.bucket ? read.ofColumn(column) : undefined
        return target && column
            ? [{ group: group.id, label: column.label, source: target.key }]
            : []
    })
}

export type FormulaDraftParts = Pick<
    BuilderDraft,
    'source' | 'columns' | 'groups' | 'params' | 'layout'
>

function formulaParams(draft: FormulaDraftParts): FormulaParam[] {
    return draft.params.map(p => ({
        id: p.id,
        label: p.label,
        type: p.type,
        multiple: p.multiple,
    }))
}

// A computed row is worked out once for every column (layout.go), each section's
// subtotal taking that column's type; one amount column makes its subtotals
// amounts, so a division of one keeps whole cents. The row reads no param itself,
// but a lookup inside it does (layout.go param, lookup.go lookupCtx.param), and it
// may look values up (F16).
function layoutContext(
    draft: FormulaDraftParts,
    catalog: Catalog,
    index: number
): ReportFormulaContext {
    const hasAmounts = columnChoices(draft, catalog).some(choice => choice.type === 'money')
    const type: FieldType = hasAmounts ? 'money' : 'number'
    const values = draft.layout.slice(0, index).map(item => ({
        key: item.id,
        label: item.kind === 'section' ? item.title : item.label,
        type,
    }))
    return {
        catalog,
        leaf: 'subtotal',
        values,
        source: null,
        records: [],
        params: [],
        lookupParams: formulaParams(draft),
        totals: false,
        windows: false,
        lookups: true,
    }
}

// The columns a level formula reads: on a level's label only those with one value
// on its rows (levels.go labelSQL); in a sort anything but a running value or a
// percent (run.go notASortKey). Elsewhere every column.
function levelOffer(
    draft: FormulaDraftParts,
    catalog: Catalog,
    place: FormulaPlace
): (key: string) => boolean {
    if (place.kind !== 'level') return () => true
    if (place.index !== undefined) {
        const shown = levelValueColumns(draft, catalog, place.index)
        return key => shown.has(key)
    }
    const windows = windowColumns(draft.columns)
    return key => !windows.has(key)
}

export function formulaContext(
    draft: FormulaDraftParts,
    catalog: Catalog,
    place: FormulaPlace
): ReportFormulaContext {
    if (place.kind === 'layout') return layoutContext(draft, catalog, place.index)
    const selfKey = place.kind === 'column' ? place.selfKey : ''
    const offered = levelOffer(draft, catalog, place)
    // Child rows have no single value on the row to read.
    const values = columnChoices(draft, catalog)
        .filter(choice => choice.key !== selfKey && !choice.isNested && offered(choice.key))
        .map(choice => ({ key: choice.key, label: choice.label, type: choice.type }))
    // A level's label reads its own group's record and outer ones, never an inner
    // group's: a level row has no one record of a group inside it (levels.go:492).
    const groups =
        place.kind === 'level' && place.index !== undefined
            ? draft.groups.slice(0, place.index + 1)
            : draft.groups
    return {
        catalog,
        leaf: 'col',
        values,
        source: draft.source,
        records: recordGroups({ ...draft, groups }, catalog),
        params: formulaParams(draft),
        totals: true,
        // A running value or a percent reads the rows a column shows (formula.go window).
        windows: place.kind === 'column',
        lookups: true,
    }
}
