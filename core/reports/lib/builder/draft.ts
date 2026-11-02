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
    aggregatesFor,
    avgScaleFor,
    type Catalog,
    DEFAULT_PIVOT_COLUMNS,
    describePath,
    MAX_COLUMNS,
    MAX_GROUP_LEVELS,
    MAX_HOPS,
    MAX_LABEL,
    MAX_LAYOUT_ITEMS,
    MAX_PARAMS,
    MAX_PIVOT_COLUMNS,
    MAX_SORT_KEYS,
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
import { columnTypes, formulaContext, isNumericType } from './formula-context'
import { isEditableNode, settleScales } from './formula-ops'
import type { ReportFormulaContext } from './formula-parts'

// The builder's form value. specFromDraft and draftFromSpec turn it into the
// engine's spec and back; the form's zod schema (builderSchema) checks what a
// person can fix before a run, and the server checks the rest.

const ID_RE = /^[A-Za-z][A-Za-z0-9_]{0,63}$/
const idSchema = z.string().regex(ID_RE, 'Use letters, digits and _, starting with a letter')
const labelSchema = z.string().trim().min(1, 'Enter a label').max(MAX_LABEL)
const AGGREGATE_CHOICES = ['', ...AGGREGATES] as const
const BUCKET_CHOICES = ['', ...BUCKETS] as const

/** A formula is the spec's own tree (R17); `kind` says whether the formula editor shows it. */
export const formulaDraftSchema = z.object({
    kind: z.enum(['editable', 'fixed']),
    node: specNodeSchema,
})
export type FormulaDraft = z.infer<typeof formulaDraftSchema>

/**
 * A stored formula: editable when the report formula editor can show it, else kept
 * as it is (R17). `params` are the report's, which tell a date range from other
 * asked values.
 */
export function formulaDraft(node: SpecNode, params: readonly ParamDef[]): FormulaDraft {
    return { kind: isEditableNode(node, params) ? 'editable' : 'fixed', node }
}

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
        .max(MAX_GROUP_LEVELS),
    sort: z
        .array(
            z.object({ col: z.string().min(1, 'Choose a column'), dir: z.enum(['asc', 'desc']) })
        )
        .max(MAX_SORT_KEYS),
    pivot: z.object({
        on: z.boolean(),
        path: z.array(specHopSchema),
        bucket: z.enum(BUCKET_CHOICES),
        cell: z.string(),
        maxColumns: z.number().int().min(1).max(MAX_PIVOT_COLUMNS),
    }),
    layout: z.array(layoutItemSchema).max(MAX_LAYOUT_ITEMS),
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

function columnDraft(
    column: SpecColumn,
    catalog: Catalog,
    baseKey: string,
    params: readonly ParamDef[]
): ColumnDraft {
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
            formula: formulaDraft(column.formula, params),
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

function layoutItemDraft(item: SpecLayoutItem, params: readonly ParamDef[]): LayoutItemDraft {
    if (item.row) {
        return {
            kind: 'row',
            id: item.row,
            label: item.label ?? item.row,
            formula: formulaDraft(item.formula ?? { value: 0 }, params),
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
    const params = spec.params ?? []
    return {
        title: meta.title,
        description: meta.description,
        source: spec.source,
        params,
        columns: spec.columns.map(column => columnDraft(column, catalog, spec.source, params)),
        filter: nodeToFilter(spec.filter, catalog, spec.source),
        groups: (spec.groups ?? []).map(g => ({
            id: g.id,
            col: g.col,
            bucket: g.bucket ?? '',
            subtotals: g.subtotals === true,
        })),
        sort: (spec.sort ?? []).map(s => ({ col: s.col ?? '', dir: s.dir ?? 'asc' })),
        pivot,
        layout: (spec.layout ?? []).map(item => layoutItemDraft(item, params)),
    }
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

// The editor settles scales on every edit; a change elsewhere (a column read becomes
// an amount) can make the engine require another, so the spec settles them again.
// A fixed formula is saved as it was read (R17).
function specFormula(formula: FormulaDraft, ctx: ReportFormulaContext): SpecNode {
    return formula.kind === 'fixed' ? formula.node : settleScales(ctx, formula.node)
}

function specColumns(draft: BuilderDraft, catalog: Catalog, types: Types): SpecColumn[] {
    const ctx = formulaContext(draft, catalog, { kind: 'column', selfKey: '' })
    return draft.columns.map(column => {
        const out: SpecColumn = { id: column.id, label: column.label }
        if (column.kind === 'path') out.path = column.path
        else out.formula = specFormula(column.formula, ctx)
        if (column.aggregate !== '') out.aggregate = column.aggregate
        if (column.aggregate === 'avg') {
            out.scale = column.scale ?? avgScaleFor(types.get(column.id) ?? 'number')
        }
        if (column.format !== '' && showsFormat(column, types)) out.format = column.format
        return out
    })
}

// "Show as" is offered on a formula column only while it gives a number; one that
// now gives text or a date saves no format it no longer shows. A fixed formula is
// kept as it was read (R17).
function showsFormat(column: ColumnDraft, types: Types): boolean {
    if (column.kind === 'path' || column.formula.kind === 'fixed') return true
    return isNumericType(types.get(column.id) ?? 'text')
}

/** The params the formulas read, in columns and layout rows. */
export function paramsReadByFormulas(draft: Pick<BuilderDraft, 'columns' | 'layout'>): Set<string> {
    const read = new Set<string>()
    for (const column of draft.columns) {
        if (column.kind === 'formula') paramsInNode(column.formula.node, read)
    }
    for (const item of draft.layout) {
        if (item.kind === 'row') paramsInNode(item.formula.node, read)
    }
    return read
}

// A param stays only while something asks for it: a filter condition or a formula.
function askedParams(draft: BuilderDraft): ParamDef[] {
    return filterParams(draft.params, draft.filter, paramsReadByFormulas(draft))
}

function specLayout(
    draft: BuilderDraft,
    catalog: Catalog,
    types: Types
): NonNullable<ReportSpec['layout']> {
    const keyType = types.get(draft.groups[0]?.col ?? '') ?? 'text'
    const ctx = formulaContext(draft, catalog, { kind: 'layout', index: draft.layout.length })
    return draft.layout.map(item =>
        item.kind === 'row'
            ? { row: item.id, label: item.label, formula: specFormula(item.formula, ctx) }
            : { section: item.id, title: item.title, key: sectionKey(item.key, keyType) }
    )
}

export function specFromDraft(draft: BuilderDraft, catalog: Catalog): ReportSpec {
    const types = columnTypes(draft, catalog)
    const spec: ReportSpec = {
        version: CURRENT_SPEC_VERSION,
        source: draft.source,
        columns: specColumns(draft, catalog, types),
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
    if (draft.layout.length > 0) spec.layout = specLayout(draft, catalog, types)
    return spec
}
