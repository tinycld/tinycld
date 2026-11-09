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

export type RowKind = 'detail' | 'group' | 'opening' | 'subtotal' | 'computed' | 'total'

export type CellValue = number | string | boolean | null

export const PARAM_TYPES = [
    'date',
    'asOf',
    'dateRange',
    'choice',
    'text',
    'number',
    'amount',
] as const
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
    pivot?: {
        cell: string
        key: unknown
        /** A pivot over a link: its column label typed, as a row's `labelType`. */
        labelType?: FieldType
        labelValue?: CellValue
        labelCurrency?: string
    }
    /**
     * A link column's cell that holds the label of the record each value points at
     * (`<id>@label`); null for a blank link or a record the reader cannot see. The
     * label is the label field's stored value, of type `labelType`, formatted as a
     * cell of that type; an amount's currency is in `labelCurrencyCell`.
     */
    labelCell?: string
    labelType?: FieldType
    labelCurrencyCell?: string
    /** The run accepts this column as a reader's sort key (options.sort). */
    sortable?: boolean
    /** What the heading shows in place of `label` this run (a day, a text or a number). */
    heading?: { value: CellValue; type: FieldType }
}

export interface ResultRow {
    id: string
    kind: RowKind
    depth: number
    /** A group or subtotal row's group level (1 for the first); absent on other rows. */
    level?: number
    label: string
    /** The column whose cell the label repeats (a child row leaves that cell out). */
    labelColumn?: string
    /** The group row this row belongs to. */
    section?: string
    /** The row a nested child row hangs under. */
    parent?: string
    groupKey?: unknown[]
    expanded?: boolean
    canExpand?: boolean
    cells: Record<string, CellValue>
    /**
     * Set when a group row's label (and its subtotal's), or a detail row's record
     * label, is not text: format
     * `labelValue` as a cell of this type (an amount in integer cents, in
     * `labelCurrency`); `label` is its plain text.
     */
    labelType?: FieldType
    labelValue?: CellValue
    labelCurrency?: string
}

export interface Notice {
    code: string
    message: string
    /**
     * The message's sentences when it has more than one, in its order (one per
     * blank column, condition or sort key it names); absent for one sentence.
     */
    parts?: string[]
    columns?: string[]
    /** The conditions (by spec path) that read a mixed-currency total. */
    conditions?: string[]
    count?: number
    leftOut?: number
    aliases?: { source: string; from: string; to: string }[]
}

export interface ResolvedParam {
    id: string
    type: ParamType
    unset?: boolean
    value?: string | number | { amount: number; currency: string }
    list?: string[]
    /** The whole groups a choice from a source picked, by group value; the run expanded them. */
    groups?: string[]
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
    /** The source's heading, printed above the title; absent: the organization's name. */
    heading?: string
    page: RunPage | null
    params: ResolvedParam[]
}

/**
 * One option of a choice param from a source: the value a run takes, its shown label
 * and its group's value ('' and absent are the blank group). A typed label (a number,
 * amount, percent or date) is formatted as a cell of `type`, an amount in `currency`.
 */
export interface ParamOption {
    value: string
    label: string
    type?: FieldType
    currency?: string
    group?: string
}

/**
 * One group of a choice param's options, its label typed as an option's is. A group
 * read from an amount has the value "<amount> <currency>" (e.g. "300 EUR").
 */
export interface ParamOptionGroup {
    value: string
    label: string
    type?: FieldType
    currency?: string
}

/** One page of a choice param's options from a source (POST /api/reports/param-options). */
export interface ParamOptionsPage {
    options: ParamOption[]
    /** The groups of every matching option; sent with the first page only. */
    groups: ParamOptionGroup[]
    /** The group list stops at the server's cap. */
    groupsCut?: boolean
    /** The cursor of the next page; absent on the last. */
    next?: string
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
