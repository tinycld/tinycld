import type { FieldType } from '../types'
import { aggregateResultType, type Catalog, describePath } from './columns'
import type { BuilderDraft, ColumnDraft } from './draft'
import type { FormulaParam, FormulaRecord, ReportFormulaContext } from './formula-parts'
import { type FormulaTypes, formulaType } from './formula-type'

// What a report formula may read where the builder edits it (OQ2): the place
// decides the leaf ({col} or {subtotal}), the values offered, and whether totals,
// running values and lookups fit there (formula.go, layout.go, lookup.go). The
// type of each column's cells is here too, since a formula reads them by id.

/**
 * Where a formula is edited: a column (`selfKey` is its own id), a layout computed
 * row (`index` is its place in the layout), a group level (its label, having, or a
 * sort formula) or a detail row's label.
 */
export type FormulaPlace =
    | { kind: 'column'; selfKey: string }
    | { kind: 'layout'; index: number }
    | { kind: 'level' }
    | { kind: 'detail' }

/**
 * What a column's type is read from: the columns, and the groups and params a
 * formula reading a record or a param is typed by. Every caller passes all four,
 * so a column has one type in every section and in the saved spec.
 */
export type ColumnsOf = Pick<BuilderDraft, 'source' | 'columns' | 'params'> & {
    groups: readonly GroupKey[]
}

/** What a group level is keyed by: enough to tell which levels carry a record. */
export type GroupKey = { id: string; col: string }

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

/** The group levels whose column is a link to one record: they carry that record. */
export function recordGroups(
    draft: Pick<BuilderDraft, 'source' | 'columns'> & { groups: readonly GroupKey[] },
    catalog: Catalog
): FormulaRecord[] {
    return draft.groups.flatMap(group => {
        const column = draft.columns.find(c => c.id === group.col)
        const info =
            column?.kind === 'path' ? describePath(catalog, draft.source, column.path) : null
        return info?.target && column
            ? [{ group: group.id, label: column.label, source: info.target.key }]
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

export function formulaContext(
    draft: FormulaDraftParts,
    catalog: Catalog,
    place: FormulaPlace
): ReportFormulaContext {
    if (place.kind === 'layout') return layoutContext(draft, catalog, place.index)
    const selfKey = place.kind === 'column' ? place.selfKey : ''
    // Child rows have no single value on the row to read.
    const values = columnChoices(draft, catalog)
        .filter(choice => choice.key !== selfKey && !choice.isNested)
        .map(choice => ({ key: choice.key, label: choice.label, type: choice.type }))
    return {
        catalog,
        leaf: 'col',
        values,
        source: draft.source,
        records: recordGroups(draft, catalog),
        params: formulaParams(draft),
        totals: true,
        // A running value or a percent reads the rows a column shows (formula.go window).
        windows: place.kind === 'column',
        lookups: true,
    }
}
