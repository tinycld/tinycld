import { z } from '@tinycld/core/ui/form'
import { AGGREGATES, PARAM_TYPES, type Problem } from './types'

// The report spec as core/server/reports/spec.go reads it. A stored spec was
// decoded strictly and saved as the engine re-encodes it, so this schema is strict
// too. Null parts read as absent: a hand-written or shipped spec may say
// "layout": null.

export const CURRENT_SPEC_VERSION = 1
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
    link?: SpecHop[]
    where?: SpecNode
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
        link: z.array(specHopSchema).optional(),
        where: specNodeSchema.optional(),
    })
)

const optionSchema = z.strictObject({ value: z.string(), label: z.string() })

export const paramDefSchema = z.strictObject({
    id: z.string(),
    type: z.enum(PARAM_TYPES),
    label: z.string().optional(),
    default: z.unknown().optional(),
    options: z.array(optionSchema).optional(),
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
})
export type SpecColumn = z.infer<typeof specColumnSchema>

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
    sort: z
        .array(z.strictObject({ col: z.string(), dir: z.enum(['asc', 'desc']).optional() }))
        .nullish(),
    groups: z
        .array(
            z.strictObject({
                id: z.string(),
                col: z.string(),
                bucket: z.enum(BUCKETS).optional(),
                subtotals: z.boolean().optional(),
            })
        )
        .nullish(),
    pivot: z
        .strictObject({
            path: z.array(specHopSchema),
            bucket: z.enum(BUCKETS).optional(),
            cell: z.string(),
            maxColumns: z.number().int().optional(),
        })
        .nullish(),
    layout: z.array(layoutItemSchema).nullish(),
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
