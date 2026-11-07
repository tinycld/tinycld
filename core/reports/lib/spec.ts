import { z } from '@tinycld/core/ui/form'
import { AGGREGATES, PARAM_TYPES, type Problem } from './types'

// The report spec as core/server/reports/spec.go reads it. A stored spec was
// decoded strictly and saved as the engine re-encodes it, so this schema is strict
// too. Null parts read as absent: a hand-written or shipped spec may say
// "layout": null.

export const CURRENT_SPEC_VERSION = 3
export const BUCKETS = ['day', 'month', 'quarter', 'year', 'fiscalYear'] as const
export type Bucket = (typeof BUCKETS)[number]

export const specHopSchema = z.strictObject({
    link: z.string().optional(),
    field: z.string().optional(),
    mode: z.enum(['aggregate', 'nest']).optional(),
    with: z.enum(['required', 'optional']).optional(),
})
export type SpecHop = z.infer<typeof specHopSchema>

export type SpecNode = {
    op?: string
    args?: SpecNode[]
    scale?: number
    col?: string
    path?: SpecHop[]
    param?: string
    value?: unknown
    subtotal?: string
    /**
     * A group whose key is a link: the leaf reads `path` from the linked record, on
     * that group's rows and every row below them.
     */
    record?: string
    link?: SpecHop[]
    where?: SpecNode
    /** The source a lookup reads; its `value` is a formula over that source. */
    source?: string
    /**
     * What a lookup takes from its matching rows' values: 'min' (the default) or
     * 'max' of them, 'sum' adds them, 'count' counts those with a value.
     */
    pick?: 'min' | 'max' | 'sum' | 'count'
    /**
     * Inside a lookup: the report row's value of this to-one path from the report's
     * own source (blank on a row with none, as a bare path is).
     */
    outer?: SpecHop[]
    /** With `param` on a date range: its first ('from') or last ('to') day. */
    bound?: 'from' | 'to'
}

// Input and output are one type, so a form schema built on this one types its values.
export const specNodeSchema: z.ZodType<SpecNode, SpecNode> = z.lazy(() =>
    z.strictObject({
        op: z.string().optional(),
        args: z.array(specNodeSchema).optional(),
        scale: z.number().int().optional(),
        col: z.string().optional(),
        path: z.array(specHopSchema).optional(),
        param: z.string().optional(),
        value: z.unknown().optional(),
        subtotal: z.string().optional(),
        record: z.string().optional(),
        link: z.array(specHopSchema).optional(),
        where: specNodeSchema.optional(),
        source: z.string().optional(),
        pick: z.enum(['min', 'max', 'sum', 'count']).optional(),
        outer: z.array(specHopSchema).optional(),
        bound: z.enum(['from', 'to']).optional(),
    })
)

const optionSchema = z.strictObject({ value: z.string(), label: z.string() })

/**
 * The order of a choice param's option groups (spec.go GroupOrder): by the group's
 * label, in the order the group's select field declares its options, or by the
 * smallest value of `path` over each group's options. The blank group comes first.
 */
export const groupOrderSchema = z.union([
    z.strictObject({ by: z.literal('label') }),
    z.strictObject({ by: z.literal('declared') }),
    z.strictObject({ by: z.literal('path'), path: z.array(specHopSchema) }),
])
export type GroupOrder = z.infer<typeof groupOrderSchema>
export type GroupOrderBy = GroupOrder['by']

/**
 * A choice param's options read from a source under the reader's list rules: `value`
 * is what the param holds, `label` what the picker shows, `group` how it groups them,
 * `groupOrder` the order of the groups (absent: by label), `groupLabel` the text of a
 * whole-group pick (`GROUP_LABEL_TOKEN` stands for the group's label; absent or empty:
 * "All {group}"), `where` which rows count.
 */
export const optionsFromSchema = z.strictObject({
    source: z.string(),
    value: z.array(specHopSchema),
    label: z.array(specHopSchema),
    group: z.array(specHopSchema).optional(),
    groupOrder: groupOrderSchema.optional(),
    groupLabel: z.string().optional(),
    where: specNodeSchema.optional(),
})

/** Stands for the group's label in `optionsFrom.groupLabel` (spec.go GroupLabelToken). */
export const GROUP_LABEL_TOKEN = '{group}'
export type OptionsFrom = z.infer<typeof optionsFromSchema>

export const paramDefSchema = z.strictObject({
    id: z.string(),
    type: z.enum(PARAM_TYPES),
    label: z.string().optional(),
    default: z.unknown().optional(),
    options: z.array(optionSchema).optional(),
    optionsFrom: optionsFromSchema.optional(),
    multiple: z.boolean().optional(),
})
export type ParamDef = z.infer<typeof paramDefSchema>

export const specColumnSchema = z.strictObject({
    id: z.string(),
    label: z.string().optional(),
    path: z.array(specHopSchema).optional(),
    formula: specNodeSchema.optional(),
    aggregate: z.enum(AGGREGATES).optional(),
    scale: z.number().int().optional(),
    format: z.string().optional(),
    // How group, subtotal and total cells are made: from every base row under them
    // (the default), or the sum of the rows the result shows directly under them.
    total: z.enum(['recompute', 'rows']).optional(),
    // The field a link column shows each record by; absent: the record's first text field.
    labelField: z.string().optional(),
    // 'blank': the column has no cell on detail rows, so a value read from the group's
    // record shows on the group row only. Absent: shown on detail rows.
    detail: z.literal('blank').optional(),
    // A formula shown on group, subtotal and total rows in place of the column's
    // totals; detail rows keep `path` or `formula`.
    rollup: specNodeSchema.optional(),
})
export type SpecColumn = z.infer<typeof specColumnSchema>

export const specSortKeySchema = z.strictObject({
    col: z.string().optional(),
    formula: specNodeSchema.optional(),
    dir: z.enum(['asc', 'desc']).optional(),
})
export type SpecSortKey = z.infer<typeof specSortKeySchema>

export const specGroupSchema = z.strictObject({
    id: z.string(),
    col: z.string(),
    bucket: z.enum(BUCKETS).optional(),
    subtotals: z.boolean().optional(),
    label: specNodeSchema.optional(),
    having: specNodeSchema.optional(),
    // Adds the linked records that have no rows, under each parent group.
    fill: z.strictObject({ where: specNodeSchema.optional() }).optional(),
    // Opens the level on a run with no expansion state (the first open).
    open: z.boolean().optional(),
    // A row first under each open group of this level: its label, and the cells
    // it shows, each a formula on the group (absent cells are blank).
    opening: z
        .strictObject({
            label: z.string().min(1).max(200),
            cells: z.record(z.string(), specNodeSchema).optional(),
        })
        .optional(),
})
export type SpecGroup = z.infer<typeof specGroupSchema>

export const DISPLAY_WIDTHS = ['mobile', 'tablet', 'desktop'] as const
export type DisplayWidth = (typeof DISPLAY_WIDTHS)[number]

// A column on `line` (from 1), at `start` (absent: the first free tracks), `span`
// tracks wide and `lines` lines tall (both 1 when absent).
export const displayCellSchema = z.strictObject({
    col: z.string(),
    line: z.number().int(),
    start: z.number().int().optional(),
    span: z.number().int().optional(),
    lines: z.number().int().optional(),
    justify: z.enum(['start', 'center', 'end']).optional(),
})
export type DisplayCell = z.infer<typeof displayCellSchema>

// The columns placed on lines of `tracks` equal tracks at one width; a column it
// does not list is not shown there.
export const displayLayoutSchema = z.strictObject({
    width: z.enum(DISPLAY_WIDTHS),
    tracks: z.number().int(),
    cells: z.array(displayCellSchema),
})
export type DisplayLayout = z.infer<typeof displayLayoutSchema>

const layoutItemSchema = z.strictObject({
    section: z.string().optional(),
    title: z.string().optional(),
    key: z.unknown().optional(),
    row: z.string().optional(),
    label: z.string().optional(),
    formula: specNodeSchema.optional(),
})

export const reportSpecSchema = z.strictObject({
    // A newer app's spec may mean something this app would read wrongly: unreadable.
    version: z.number().int().min(1).max(CURRENT_SPEC_VERSION),
    source: z.string().min(1),
    params: z.array(paramDefSchema).nullish(),
    columns: z.array(specColumnSchema),
    filter: specNodeSchema.nullish(),
    sort: z.array(specSortKeySchema).nullish(),
    groups: z.array(specGroupSchema).nullish(),
    pivot: z
        .strictObject({
            path: z.array(specHopSchema),
            bucket: z.enum(BUCKETS).optional(),
            cell: z.string(),
            maxColumns: z.number().int().optional(),
            labelField: z.string().optional(),
        })
        .nullish(),
    layout: z.array(layoutItemSchema).nullish(),
    // A formula on each detail row that labels it (the first column's value without it).
    detail: z.strictObject({ label: specNodeSchema.optional() }).nullish(),
    display: z.strictObject({ layouts: z.array(displayLayoutSchema).optional() }).nullish(),
})
export type ReportSpec = z.infer<typeof reportSpecSchema>

export function parseSpec(value: unknown): ReportSpec | null {
    const parsed = reportSpecSchema.safeParse(value)
    return parsed.success ? parsed.data : null
}

// reports.broken: why a saved report cannot run (core/server/reports/broken.go).
export const brokenSchema = z.object({
    status: z.number(),
    problems: z.record(z.string(), z.object({ code: z.string(), message: z.string() })),
    more: z.number().optional(),
})
export type BrokenMark = z.infer<typeof brokenSchema>

export function parseBroken(value: unknown): BrokenMark | null {
    const parsed = brokenSchema.safeParse(value)
    return parsed.success ? parsed.data : null
}

// The server refuses to copy a report that cannot run, so a copy is offered only for a
// report with no broken mark and a spec this app can read.
export function canCopyReport(report: { spec: unknown; broken: unknown }): boolean {
    return parseBroken(report.broken) === null && parseSpec(report.spec) !== null
}

export function brokenProblems(mark: BrokenMark): Problem[] {
    return Object.entries(mark.problems)
        .map(([path, problem]) => ({ path, code: problem.code, message: problem.message }))
        .sort((a, b) => a.path.localeCompare(b.path))
}
