// The run endpoint's answer and the source catalog, as core/server/reports sends
// them (run.go Result, catalog.go CatalogSource). Money cells are integer cents;
// a percent cell is already a percent (12.5 means 12.5%).

/**
 * The types a bucketed group row's or pivot column's label carries (registry.go
 * Month … Day), with the raw bucket key as its value. Never a field's type.
 */
export const BUCKET_LABEL_TYPES = ['month', 'quarter', 'year', 'fiscalYear', 'day'] as const
export type BucketLabelType = (typeof BUCKET_LABEL_TYPES)[number]

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
    ...BUCKET_LABEL_TYPES,
] as const
export type FieldType = (typeof FIELD_TYPES)[number]

const BUCKET_LABEL_SET: ReadonlySet<FieldType> = new Set(BUCKET_LABEL_TYPES)

export function isBucketLabelType(type: FieldType): type is BucketLabelType {
    return BUCKET_LABEL_SET.has(type)
}

export const AGGREGATES = ['sum', 'count', 'countDistinct', 'min', 'max', 'avg'] as const
export type Aggregate = (typeof AGGREGATES)[number]

export type RowKind = 'detail' | 'group' | 'opening' | 'subtotal' | 'computed' | 'total'

export type CellValue = number | string | boolean | null

/** The report a drilled column's figures open (run.go DrillTarget). */
export interface DrillTarget {
    reportId: string
    title: string
}

/**
 * A `<col>@drill` cell: the target's param values by param id, each in the run
 * request's JSON form. Read it only through `drillCellParams` (drill.ts): cells
 * stay typed as `CellValue`, which every other cell is (R12).
 */
export type DrillParams = Record<string, unknown>

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
    /**
     * The decimal places a percent part's column (percentOfTotal, percentOfParent,
     * percentOfSection) prints, 0 included; none means a percent prints 1 place.
     */
    scale?: number
    options?: SelectOption[]
    /** A money column's fixed currency. */
    currency?: string
    /** The cell that holds each row's currency instead (`<id>@currency`). */
    currencyCell?: string
    mixedCurrency?: boolean
    pivot?: {
        cell: string
        key: unknown
        /** The column label typed, as a row's `labelType`: a pivot over a link's record label, an amount, or a bucket (month … day) with the raw key. */
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
    /**
     * The column has no cell above a group level (the engine's blankAbove):
     * column-measure.ts widestCell keeps room for its worst case.
     */
    blankAbove?: boolean
    /** What the heading shows in place of `label` this run (a day, a text or a number). */
    heading?: { value: CellValue; type: FieldType }
    /** The report this column's figures open; absent when it has none or the reader may not view it. */
    drill?: DrillTarget
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
    /** The app path a detail row opens (spec `detail.open`). */
    open?: string
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
    /**
     * A group row its level's `muted` condition holds for: its label and figures are
     * drawn in muted text. Never set on a subtotal or total row.
     */
    muted?: boolean
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
    /**
     * The lines printed under the period: one per choice param with a `header`, in
     * param order, with the chosen option's label; '' for a param that is unset.
     */
    headerLines: string[]
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
