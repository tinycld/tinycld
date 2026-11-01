// The run endpoint's answer and the source catalog, as core/server/reports sends
// them (run.go Result, catalog.go CatalogSource). Money cells are integer cents;
// a percent cell is already a percent (12.5 means 12.5%).

export const FIELD_TYPES = [
    'text',
    'number',
    'money',
    'date',
    'datetime',
    'bool',
    'select',
    'relation',
    'percent',
] as const
export type FieldType = (typeof FIELD_TYPES)[number]

export const AGGREGATES = ['sum', 'count', 'countDistinct', 'min', 'max', 'avg'] as const
export type Aggregate = (typeof AGGREGATES)[number]

export type RowKind = 'detail' | 'group' | 'subtotal' | 'computed' | 'total'

export type CellValue = number | string | boolean | null

export const PARAM_TYPES = ['date', 'asOf', 'dateRange', 'choice', 'text', 'number'] as const
export type ParamType = (typeof PARAM_TYPES)[number]

export interface SelectOption {
    value: string
    label: string
}

export interface ResultColumn {
    id: string
    label: string
    type: FieldType
    /** The type of group, subtotal and total cells when it differs (a count of names). */
    totalType?: FieldType
    aggregate?: Aggregate
    format?: string
    options?: SelectOption[]
    /** A money column's fixed currency. */
    currency?: string
    /** The cell that holds each row's currency instead (`<id>@currency`). */
    currencyCell?: string
    mixedCurrency?: boolean
    pivot?: { cell: string; key: unknown }
}

export interface ResultRow {
    id: string
    kind: RowKind
    depth: number
    label: string
    /** The group row this row belongs to. */
    section?: string
    /** The row a nested child row hangs under. */
    parent?: string
    groupKey?: unknown[]
    expanded?: boolean
    canExpand?: boolean
    cells: Record<string, CellValue>
}

export interface Notice {
    code: string
    message: string
    columns?: string[]
    count?: number
    leftOut?: number
    /** The spec version an `upgraded` notice came from. */
    fromVersion?: number
    aliases?: { source: string; from: string; to: string }[]
}

export interface ResolvedParam {
    id: string
    type: ParamType
    unset?: boolean
    value?: string | number
    list?: string[]
    from?: string
    to?: string
}

export interface RunPage {
    next?: string
    size: number
}

export interface RunResult {
    columns: ResultColumn[]
    rows: ResultRow[]
    notices: Notice[]
    topics: string[]
    page: RunPage | null
    params: ResolvedParam[]
}

export interface CatalogField {
    key: string
    label: string
    type: FieldType
    aggregates: Aggregate[]
    options?: SelectOption[]
    /** The key of the field that holds each row's currency. */
    currency?: string
}

export interface CatalogLink {
    key: string
    label: string
    target: string
    cardinality: 'toOne' | 'toMany'
}

export interface CatalogSource {
    key: string
    label: string
    fields: CatalogField[]
    links: CatalogLink[]
}

/** One refused part of a spec, keyed by its dotted spec path (`columns.0.id`). */
export interface Problem {
    path: string
    code: string
    message: string
}
