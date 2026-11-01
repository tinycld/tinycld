import { type ComputeExpr, computeExprSchema } from '@tinycld/core/lib/forms/expression'
import { z } from '@tinycld/core/ui/form'
import {
    BUCKETS,
    CURRENT_SPEC_VERSION,
    type ParamDef,
    paramDefSchema,
    type ReportSpec,
    type SpecColumn,
    type SpecNode,
    specHopSchema,
    specNodeSchema,
} from '../spec'
import { AGGREGATES, type CatalogSource, type FieldType } from '../types'
import {
    aggregateResultType,
    aggregatesFor,
    avgScaleFor,
    type Catalog,
    DEFAULT_PIVOT_COLUMNS,
    describePath,
    MAX_COLUMNS,
    MAX_HOPS,
    MAX_LABEL,
    MAX_PARAMS,
    MAX_PIVOT_COLUMNS,
    nextColumnId,
    pathLabel,
} from './columns'
import {
    conditionProblems,
    emptyGroup,
    filterGroupSchema,
    filterParams,
    filterToNode,
    nodeToFilter,
    paramsInNode,
} from './filter'
import { type FormulaLeaf, fromComputeExpr, isAmountExpr, toComputeExpr } from './formula'

// The builder's form value. specFromDraft and draftFromSpec turn it into the
// engine's spec and back; the form's zod schema (builderSchema) checks what a
// person can fix before a run, and the server checks the rest.

const ID_RE = /^[A-Za-z][A-Za-z0-9_]{0,63}$/
const idSchema = z.string().regex(ID_RE, 'Use letters, digits and _, starting with a letter')
const labelSchema = z.string().trim().min(1, 'Enter a label').max(MAX_LABEL)
const AGGREGATE_CHOICES = ['', ...AGGREGATES] as const
const BUCKET_CHOICES = ['', ...BUCKETS] as const
// computeExprSchema reads unknown input; the form needs one type in and out.
const exprSchema = z.custom<ComputeExpr>(
    value => computeExprSchema.safeParse(value).success,
    'This formula is not valid'
)

export const formulaDraftSchema = z.union([
    z.object({ kind: z.literal('editable'), expr: exprSchema }),
    z.object({ kind: z.literal('fixed'), node: specNodeSchema }),
])
export type FormulaDraft = z.infer<typeof formulaDraftSchema>

const columnFields = {
    id: idSchema,
    label: labelSchema,
    aggregate: z.enum(AGGREGATE_CHOICES),
    /** An average's decimal places; null takes the field type's default. */
    scale: z.number().int().min(0).max(6).nullable(),
    /** The client's display format, kept as given ('percent' is the one the builder offers). */
    format: z.string().max(50),
}
export const columnDraftSchema = z.union([
    z.object({
        kind: z.literal('path'),
        ...columnFields,
        path: z
            .array(specHopSchema)
            .min(1)
            .max(MAX_HOPS + 1),
    }),
    z.object({ kind: z.literal('formula'), ...columnFields, formula: formulaDraftSchema }),
])
export type ColumnDraft = z.infer<typeof columnDraftSchema>

const layoutItemSchema = z.union([
    z.object({
        kind: z.literal('section'),
        id: idSchema,
        title: labelSchema,
        key: z.string().min(1, 'Choose the group this section shows'),
    }),
    z.object({
        kind: z.literal('row'),
        id: idSchema,
        label: labelSchema,
        formula: formulaDraftSchema,
    }),
])
export type LayoutItemDraft = z.infer<typeof layoutItemSchema>

const draftShape = z.object({
    title: z.string().trim().min(1, 'Enter a title').max(200),
    description: z.string().max(2000),
    source: z.string().min(1),
    params: z.array(paramDefSchema).max(MAX_PARAMS),
    columns: z.array(columnDraftSchema).min(1, 'Add a column').max(MAX_COLUMNS),
    filter: filterGroupSchema,
    groups: z
        .array(
            z.object({
                id: idSchema,
                col: z.string().min(1, 'Choose a column'),
                bucket: z.enum(BUCKET_CHOICES),
                subtotals: z.boolean(),
            })
        )
        .max(6),
    sort: z
        .array(
            z.object({ col: z.string().min(1, 'Choose a column'), dir: z.enum(['asc', 'desc']) })
        )
        .max(10),
    pivot: z.object({
        on: z.boolean(),
        path: z.array(specHopSchema),
        bucket: z.enum(BUCKET_CHOICES),
        cell: z.string(),
        maxColumns: z.number().int().min(1).max(MAX_PIVOT_COLUMNS),
    }),
    layout: z.array(layoutItemSchema).max(200),
})
export type BuilderDraft = z.infer<typeof draftShape>

type Issue = { path: (string | number)[]; message: string }

function columnIssues(draft: BuilderDraft, catalog: Catalog): Issue[] {
    const issues: Issue[] = []
    const ids = draft.columns.map(column => column.id)
    draft.columns.forEach((column, i) => {
        if (ids.indexOf(column.id) !== i) {
            issues.push({ path: ['columns', i, 'id'], message: 'Two columns have this id' })
        }
        if (column.kind !== 'path') return
        const info = describePath(catalog, draft.source, column.path)
        if (!info) {
            issues.push({
                path: ['columns', i, 'label'],
                message: 'This field is no longer offered',
            })
            return
        }
        if (info.mode === 'aggregate' && column.aggregate === '') {
            issues.push({ path: ['columns', i, 'aggregate'], message: 'Choose how to total it' })
        } else if (
            column.aggregate !== '' &&
            !aggregatesFor(info.field, info.viaToMany).includes(column.aggregate)
        ) {
            issues.push({
                path: ['columns', i, 'aggregate'],
                message: 'This total is not offered for this field',
            })
        }
    })
    return issues
}

function groupingIssues(draft: BuilderDraft, catalog: Catalog): Issue[] {
    const issues: Issue[] = []
    const ids = draft.columns.map(column => column.id)
    draft.groups.forEach((group, i) => {
        if (group.col !== '' && !ids.includes(group.col)) {
            issues.push({ path: ['groups', i, 'col'], message: 'Choose a column' })
        }
    })
    draft.sort.forEach((key, i) => {
        if (key.col !== '' && !ids.includes(key.col)) {
            issues.push({ path: ['sort', i, 'col'], message: 'Choose a column' })
        }
    })
    if (draft.pivot.on) {
        if (draft.groups.length === 0)
            issues.push({ path: ['pivot', 'on'], message: 'A pivot needs a group' })
        if (!describePath(catalog, draft.source, draft.pivot.path)) {
            issues.push({ path: ['pivot', 'path'], message: 'Choose a field' })
        }
        const cell = draft.columns.find(column => column.id === draft.pivot.cell)
        if (!cell || cell.aggregate === '') {
            issues.push({ path: ['pivot', 'cell'], message: 'Choose a column with a total' })
        }
    }
    if (draft.layout.length > 0 && draft.groups.length === 0) {
        issues.push({ path: ['layout'], message: 'A layout needs a group' })
    }
    const layoutIds = draft.layout.map(item => item.id)
    draft.layout.forEach((item, i) => {
        if (layoutIds.indexOf(item.id) !== i) {
            issues.push({ path: ['layout', i, 'id'], message: 'Two layout items have this id' })
        }
    })
    return issues
}

export function draftIssues(draft: BuilderDraft, catalog: Catalog): Issue[] {
    const issues = [...columnIssues(draft, catalog), ...groupingIssues(draft, catalog)]
    const [firstProblem] = conditionProblems(draft.filter, catalog, draft.source)
    if (firstProblem) issues.push({ path: ['filter'], message: firstProblem })
    return issues
}

export function builderSchema(catalog: Catalog) {
    return draftShape.superRefine((draft, ctx) => {
        for (const issue of draftIssues(draft, catalog)) {
            ctx.addIssue({ code: 'custom', path: issue.path, message: issue.message })
        }
    })
}

export function emptyDraft(source: CatalogSource): BuilderDraft {
    const first = source.fields.find(field => field.key !== 'id') ?? source.fields[0]
    const columns: ColumnDraft[] = first
        ? [
              {
                  kind: 'path',
                  id: nextColumnId([], first.key),
                  label: first.label,
                  path: [{ field: first.key }],
                  aggregate: '',
                  scale: null,
                  format: '',
              },
          ]
        : []
    return {
        title: '',
        description: '',
        source: source.key,
        params: [],
        columns,
        filter: emptyGroup(),
        groups: [],
        sort: [],
        pivot: { on: false, path: [], bucket: '', cell: '', maxColumns: DEFAULT_PIVOT_COLUMNS },
        layout: [],
    }
}

function formulaDraft(node: SpecNode, leaf: FormulaLeaf): FormulaDraft {
    const expr = toComputeExpr(node, leaf)
    return expr === null ? { kind: 'fixed', node } : { kind: 'editable', expr }
}

function columnDraft(column: SpecColumn, catalog: Catalog, baseKey: string): ColumnDraft {
    const common = {
        id: column.id,
        aggregate: column.aggregate ?? '',
        scale: column.scale ?? null,
        format: column.format ?? '',
    } as const
    if (column.formula) {
        return {
            kind: 'formula',
            ...common,
            label: column.label ?? column.id,
            formula: formulaDraft(column.formula, 'col'),
        }
    }
    const path = column.path ?? []
    const info = describePath(catalog, baseKey, path)
    return {
        kind: 'path',
        ...common,
        label: column.label ?? (info ? pathLabel(info) : column.id),
        path,
    }
}

type SpecLayoutItem = NonNullable<ReportSpec['layout']>[number]

function layoutItemDraft(item: SpecLayoutItem): LayoutItemDraft {
    if (item.row) {
        return {
            kind: 'row',
            id: item.row,
            label: item.label ?? item.row,
            formula: formulaDraft(item.formula ?? { value: 0 }, 'subtotal'),
        }
    }
    return {
        kind: 'section',
        id: item.section ?? '',
        title: item.title ?? '',
        key: String(item.key ?? ''),
    }
}

export function draftFromSpec(
    spec: ReportSpec,
    meta: { title: string; description: string },
    catalog: Catalog
): BuilderDraft {
    const pivot: BuilderDraft['pivot'] = spec.pivot
        ? {
              on: true,
              path: spec.pivot.path,
              bucket: spec.pivot.bucket ?? '',
              cell: spec.pivot.cell,
              maxColumns: spec.pivot.maxColumns ?? DEFAULT_PIVOT_COLUMNS,
          }
        : { on: false, path: [], bucket: '', cell: '', maxColumns: DEFAULT_PIVOT_COLUMNS }
    return {
        title: meta.title,
        description: meta.description,
        source: spec.source,
        params: spec.params ?? [],
        columns: spec.columns.map(column => columnDraft(column, catalog, spec.source)),
        filter: nodeToFilter(spec.filter, catalog, spec.source),
        groups: (spec.groups ?? []).map(g => ({
            id: g.id,
            col: g.col,
            bucket: g.bucket ?? '',
            subtotals: g.subtotals === true,
        })),
        sort: (spec.sort ?? []).map(s => ({ col: s.col, dir: s.dir ?? 'asc' })),
        pivot,
        layout: (spec.layout ?? []).map(layoutItemDraft),
    }
}

type ColumnsOf = Pick<BuilderDraft, 'source' | 'columns'>

/**
 * The type of each column's cells, by id, for formula operands, division and
 * average scales, pivot cells and layout keys. An editable formula is an amount
 * when the engine would type it so from the columns it reads; a fixed formula
 * reads as a number.
 */
function columnTypes(draft: ColumnsOf, catalog: Catalog): ReadonlyMap<string, FieldType> {
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
    const readsAmount = (key: string) => {
        const read = byId.get(key)
        return read ? typeOf(read) === 'money' : false
    }
    const cellType = (column: ColumnDraft): FieldType => {
        if (column.kind === 'path') {
            const info = describePath(catalog, draft.source, column.path)
            return info ? aggregateResultType(column.aggregate, info.field.type) : 'text'
        }
        if (column.formula.kind === 'fixed') return 'number'
        const type = isAmountExpr(column.formula.expr, readsAmount) ? 'money' : 'number'
        return aggregateResultType(column.aggregate, type)
    }
    for (const column of draft.columns) typeOf(column)
    return types
}

export function columnIsAmount(column: ColumnDraft, draft: ColumnsOf, catalog: Catalog): boolean {
    return columnTypes(draft, catalog).get(column.id) === 'money'
}

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
            isNumeric: type === 'number' || type === 'money' || type === 'percent',
            hasAggregate: column.aggregate !== '',
            isDate: type === 'date' || type === 'datetime',
            isNested: info?.mode === 'nest',
            options: info?.field.options ?? [],
        }
    })
}

function formulaNode(
    formula: FormulaDraft,
    leaf: FormulaLeaf,
    isAmount: (key: string) => boolean
): SpecNode {
    return formula.kind === 'fixed' ? formula.node : fromComputeExpr(formula.expr, leaf, isAmount)
}

function sectionKey(text: string, type: FieldType): unknown {
    if (
        (type === 'number' || type === 'money') &&
        text.trim() !== '' &&
        Number.isFinite(Number(text))
    ) {
        return Number(text)
    }
    if (type === 'bool') return text === 'true'
    return text
}

type Types = ReadonlyMap<string, FieldType>

function specColumns(draft: BuilderDraft, types: Types): SpecColumn[] {
    const isAmount = (key: string) => types.get(key) === 'money'
    return draft.columns.map(column => {
        const out: SpecColumn = { id: column.id, label: column.label }
        if (column.kind === 'path') out.path = column.path
        else out.formula = formulaNode(column.formula, 'col', isAmount)
        if (column.aggregate !== '') out.aggregate = column.aggregate
        if (column.aggregate === 'avg') {
            out.scale = column.scale ?? avgScaleFor(types.get(column.id) ?? 'number')
        }
        if (column.format !== '') out.format = column.format
        return out
    })
}

/** The params the columns read: only a fixed formula can (an editable one cannot, nor can a layout row). */
export function paramsReadByColumns(columns: readonly ColumnDraft[]): Set<string> {
    const read = new Set<string>()
    for (const column of columns) {
        if (column.kind === 'formula' && column.formula.kind === 'fixed') {
            paramsInNode(column.formula.node, read)
        }
    }
    return read
}

// A param stays only while something asks for it: a filter condition or a column.
function askedParams(draft: BuilderDraft): ParamDef[] {
    return filterParams(draft.params, draft.filter, paramsReadByColumns(draft.columns))
}

function specLayout(draft: BuilderDraft, types: Types): NonNullable<ReportSpec['layout']> {
    const keyType = types.get(draft.groups[0]?.col ?? '') ?? 'text'
    // The engine computes a layout row once for every number column and pivot
    // cell (layout.go), each subtotal taking that column's type; one money
    // column makes a division need whole cents.
    const hasAmounts = draft.columns.some(column => types.get(column.id) === 'money')
    const isAmount = () => hasAmounts
    return draft.layout.map(item =>
        item.kind === 'row'
            ? {
                  row: item.id,
                  label: item.label,
                  formula: formulaNode(item.formula, 'subtotal', isAmount),
              }
            : { section: item.id, title: item.title, key: sectionKey(item.key, keyType) }
    )
}

export function specFromDraft(draft: BuilderDraft, catalog: Catalog): ReportSpec {
    const types = columnTypes(draft, catalog)
    const spec: ReportSpec = {
        version: CURRENT_SPEC_VERSION,
        source: draft.source,
        columns: specColumns(draft, types),
    }
    const params = askedParams(draft)
    if (params.length > 0) spec.params = params
    const filter = filterToNode(draft.filter, catalog, draft.source)
    if (filter) spec.filter = filter
    if (draft.sort.length > 0) {
        spec.sort = draft.sort.map(s =>
            s.dir === 'desc' ? { col: s.col, dir: 'desc' } : { col: s.col }
        )
    }
    if (draft.groups.length > 0) {
        spec.groups = draft.groups.map(g => ({
            id: g.id,
            col: g.col,
            ...(g.bucket !== '' ? { bucket: g.bucket } : {}),
            ...(g.subtotals ? { subtotals: true } : {}),
        }))
    }
    if (draft.pivot.on) {
        spec.pivot = {
            path: draft.pivot.path,
            ...(draft.pivot.bucket !== '' ? { bucket: draft.pivot.bucket } : {}),
            cell: draft.pivot.cell,
            maxColumns: draft.pivot.maxColumns,
        }
    }
    if (draft.layout.length > 0) spec.layout = specLayout(draft, types)
    return spec
}
