import type { ReportBuilder } from '../../hooks/useReportBuilder'
import {
    AGGREGATE_LABELS,
    aggregatesFor,
    type Catalog,
    describePath,
    pathLabel,
} from '../../lib/builder/columns'
import {
    type AboveGroup,
    type ConditionDraft,
    emptyCondition,
    type FilterPath,
    type GroupDraft,
    type LevelScope,
    opsFor,
    updateFilterItem,
} from '../../lib/builder/filter'
import type { FormulaRecord } from '../../lib/builder/formula-parts'
import type { ParamDef, SpecHop } from '../../lib/spec'
import type { Aggregate, CatalogField, CatalogSource, FieldType } from '../../lib/types'

// What the condition tree's parts share (ConditionTree, ConditionSubjects,
// ConditionValues): the editing context, and what a condition compares.

/**
 * A group above a fill's level, as its column's cells are typed: a value is its key
 * only when it is of the same kind (the same type, the same linked source, the same
 * options).
 */
export type AboveChoice = AboveGroup & {
    type: FieldType
    target?: string
    options?: readonly string[]
}

// What every part of the tree editor needs: the tree, the asked params, and how to
// change them. Every change replaces the whole tree (and the params, R20) in one
// edit: a param nothing reads any more leaves with it.
export interface FilterContext {
    builder: ReportBuilder
    catalog: Catalog
    root: GroupDraft
    params: ParamDef[]
    testID: string
    level: LevelScope | undefined
    group: string
    above: readonly AboveChoice[]
    change(next: GroupDraft, ...added: ParamDef[]): void
}

export function itemTestId(ctx: FilterContext, path: FilterPath): string {
    return path.length === 0 ? ctx.testID : `${ctx.testID}-${path.join('-')}`
}

export function setCondition(
    ctx: FilterContext,
    path: FilterPath,
    patch: Partial<ConditionDraft>,
    ...added: ParamDef[]
) {
    ctx.change(
        updateFilterItem(ctx.root, path, item =>
            item.kind === 'condition' ? { ...item, ...patch } : item
        ),
        ...added
    )
}

export const typeAt = (catalog: Catalog, baseKey: string, path: SpecHop[]) =>
    describePath(catalog, baseKey, path)?.field.type ?? 'text'

/** A new condition starts with its type's first operator. */
export function startCondition(
    type: FieldType,
    parts: Partial<ConditionDraft> & { path: SpecHop[] }
): ConditionDraft {
    return { ...emptyCondition(parts.path), ...parts, op: opsFor(type)[0] ?? 'eq' }
}

export const isCount = (aggregate: Aggregate | '') =>
    aggregate === '' || aggregate === 'count' || aggregate === 'countDistinct'

/**
 * What a condition compares, as a field: its label, its type and options, and the
 * source a relation points at. A total and a column read as fields of their own.
 */
export interface ConditionInfo {
    label: string
    field: CatalogField
    target?: CatalogSource
    /** The field a total adds up, or a record's field: what its picker shows. */
    pathText: string
}

export function recordOf(ctx: FilterContext, item: ConditionDraft): FormulaRecord | undefined {
    return item.subject === 'record'
        ? ctx.level?.records.find(r => r.group === item.record)
        : undefined
}

export function conditionInfo(
    ctx: FilterContext,
    item: ConditionDraft,
    baseKey: string
): ConditionInfo | null {
    if (item.subject === 'column') {
        const col = ctx.level?.columns.find(c => c.key === item.col)
        return col
            ? { label: col.label, field: col.field, target: col.target, pathText: col.label }
            : null
    }
    const record = recordOf(ctx, item)
    if (item.subject === 'record' && !record) return null
    const info = describePath(ctx.catalog, record?.source ?? baseKey, item.path)
    if (!info) return null
    const pathText = pathLabel(info)
    if (item.subject === 'total') {
        const label = `${AGGREGATE_LABELS[item.aggregate || 'count']} of ${pathText}`
        const type = isCount(item.aggregate) ? 'number' : info.field.type
        const options = isCount(item.aggregate) ? undefined : info.field.options
        const field = { key: info.field.key, label, type, aggregates: [], options }
        return { label, field, pathText }
    }
    const label = record ? `${record.label} › ${pathText}` : pathText
    return { label, field: info.field, target: info.target, pathText }
}

export function newTotal(ctx: FilterContext, baseKey: string, path: SpecHop[]): ConditionDraft {
    const field = describePath(ctx.catalog, baseKey, path)?.field
    const offered = field ? aggregatesFor(field, false) : []
    const adds = field && (field.type === 'money' || field.type === 'number')
    const aggregate: Aggregate = adds && offered.includes('sum') ? 'sum' : 'count'
    return { ...emptyCondition(path), subject: 'total', aggregate, op: 'gt' }
}

export function newColumnCondition(column: LevelScope['columns'][number]): ConditionDraft {
    return startCondition(column.field.type, { path: [], subject: 'column', col: column.key })
}

export function newRecordCondition(
    ctx: FilterContext,
    record: FormulaRecord,
    path: SpecHop[]
): ConditionDraft {
    const type = typeAt(ctx.catalog, record.source, path)
    return startCondition(type, { path, subject: 'record', record: record.group })
}
