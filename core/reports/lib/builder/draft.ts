import { z } from '@tinycld/core/ui/form'
import { isFigureType } from '../column-measure'
import { utf8Length } from '../params'
import {
    BUCKETS,
    type ColumnFit,
    CURRENT_SPEC_VERSION,
    columnFitSchema,
    DISPLAY_WIDTHS,
    type DisplayLayout,
    displayLayoutSchema,
    displayOneLineSchema,
    GROUP_LABEL_TOKEN,
    type ParamDef,
    paramDefSchema,
    type ReportSpec,
    type SpecColumn,
    type SpecGroup,
    type SpecNode,
    type SpecSortKey,
    specHopSchema,
    specNodeSchema,
} from '../spec'
import { MAX_TRACKS, placeCells } from '../table-lines'
import { AGGREGATES, type CatalogSource, type FieldType } from '../types'
import {
    aggregatesFor,
    avgScaleFor,
    type Catalog,
    DEFAULT_PIVOT_COLUMNS,
    describePath,
    firstTextField,
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
    type AboveGroup,
    aboveOf,
    conditionProblems,
    emptyGroup,
    filterGroupSchema,
    filterToNode,
    groupOrderChoices,
    type LevelScope,
    nodeToFilter,
    paramsInGroup,
    paramsInNode,
    tidyParam,
} from './filter'
import {
    type ColumnsOf,
    columnChoices,
    columnTarget,
    columnTypes,
    formulaContext,
    isNumericType,
    levelValueColumns,
    readsSubtotal,
    recordGroups,
    sectionPercentColumns,
    type TargetDraft,
} from './formula-context'
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

/** "Start each group with a row" (F5): its label and a formula per column it fills. */
export const openingDraftSchema = z.object({
    label: labelSchema,
    cells: z.record(z.string(), formulaDraftSchema),
})
export type OpeningDraft = z.infer<typeof openingDraftSchema>

export function openingDraft(label: string): OpeningDraft {
    return { label, cells: {} }
}

/** The opening row with column `col` showing `formula`, or no value there when null. */
export function setOpeningCell(
    opening: OpeningDraft,
    col: string,
    formula: FormulaDraft | null
): OpeningDraft {
    const kept = Object.entries(opening.cells).filter(([key]) => key !== col)
    const cells = formula ? [...kept, [col, formula] as const] : kept
    return { ...opening, cells: Object.fromEntries(cells) }
}

/**
 * The cells an opening row's editor lists: each own-row column after the first
 * (bind.go opening: the label fills the first, a child row column has none), then
 * each cell Save refuses (its column gone, first or of child rows), stale, so it
 * stays in view to be removed.
 */
export function openingCellRows(columns: readonly ColumnDraft[], opening: OpeningDraft) {
    const offered = columns
        .slice(1)
        .filter(column => nestDepth(column) === 0)
        .map(column => ({ col: column.id, label: column.label, isStale: false }))
    const offeredIds = new Set(offered.map(row => row.col))
    const stale = Object.keys(opening.cells)
        .filter(col => !offeredIds.has(col))
        .map(col => ({
            col,
            label: columns.find(column => column.id === col)?.label ?? col,
            isStale: true,
        }))
    return [...offered, ...stale]
}

const columnFields = {
    id: idSchema,
    label: labelSchema,
    aggregate: z.enum(AGGREGATE_CHOICES),
    /** An average's decimal places; null takes the field type's default. */
    scale: z.number().int().min(0).max(6).nullable(),
    /** The client's display format, kept as given ('percent' is the one the builder offers). */
    format: z.string().max(50),
    /** Level and total cells: recomputed from every row, or the rows shown added (E2). */
    total: z.enum(['recompute', 'rows']),
    /** The read spec said `total` itself, so a save keeps even the default. */
    explicitTotal: z.boolean(),
    /** The field a link column shows its record by; '' takes the first text field (R31). */
    labelField: z.string(),
    /** On opened (detail) rows: the column's value, or left blank; absent reads as shown. */
    detail: z.enum(['show', 'blank']).optional(),
    /** On group, subtotal and total rows: a formula instead of the column's total (F6). */
    rollup: formulaDraftSchema.nullable().optional(),
    /** The column's fit (display.columns), kept as the spec says it; a save tidies it (tidyFit). */
    fit: columnFitSchema.optional(),
    /** The heading from a formula over params (a day); null or absent: the label. */
    heading: formulaDraftSchema.nullable().optional(),
    /** Computed to group or sort by, never shown (G7). */
    hidden: z.boolean().optional(),
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
        /** null: only the columns `cells` names have a figure. */
        formula: formulaDraftSchema.nullable(),
        /** The earlier section the row sits inside; absent or '': after the sections. */
        within: z.string().optional(),
        /** The row's own formula in a column, by column id. */
        cells: z.record(z.string(), formulaDraftSchema).optional(),
    }),
])
export type LayoutItemDraft = z.infer<typeof layoutItemSchema>

const sortDir = {
    dir: z.enum(['asc', 'desc']),
    /** The read spec said `dir` itself, so a save keeps even 'asc'. */
    explicitDir: z.boolean(),
}
/** A sort key: a column, or a formula (a group record's field, R9). */
export const sortDraftSchema = z.union([
    z.object({ kind: z.literal('col'), col: z.string().min(1, 'Choose a column'), ...sortDir }),
    z.object({ kind: z.literal('formula'), formula: formulaDraftSchema, ...sortDir }),
])
export type SortDraft = z.infer<typeof sortDraftSchema>

export const groupLevelSchema = z.object({
    id: idSchema,
    col: z.string().min(1, 'Choose a column'),
    bucket: z.enum(BUCKET_CHOICES),
    subtotals: z.boolean(),
    /** Opens the level when the report opens. */
    open: z.boolean(),
    /** null: the key, or a record group's first text field, labels the row (R9). */
    label: formulaDraftSchema.nullable(),
    /** Show only groups where…: conditions on the level's totals, columns and record (R18). */
    having: filterGroupSchema,
    /** Show every linked record, even with no rows, of those `where` keeps (R6). */
    fill: z.object({ on: z.boolean(), where: filterGroupSchema }),
    /** The row that starts each opened group of this level (F5); none when null or absent. */
    opening: openingDraftSchema.nullable().optional(),
    /** Rows with no key: a '(none)' group, or directly under the group above. */
    blank: z.enum(['group', 'skip']).optional(),
    /** Hide a group when every row in it is hidden (G5). */
    hideEmpty: z.boolean().optional(),
})
export type GroupLevelDraft = z.infer<typeof groupLevelSchema>

const draftShape = z.object({
    title: z.string().trim().min(1, 'Enter a title').max(200),
    description: z.string().max(2000),
    source: z.string().min(1),
    params: z.array(paramDefSchema).max(MAX_PARAMS),
    columns: z.array(columnDraftSchema).min(1, 'Add a column').max(MAX_COLUMNS),
    filter: filterGroupSchema,
    groups: z.array(groupLevelSchema).max(MAX_GROUP_LEVELS),
    sort: z.array(sortDraftSchema).max(MAX_SORT_KEYS),
    pivot: z.object({
        on: z.boolean(),
        path: z.array(specHopSchema),
        bucket: z.enum(BUCKET_CHOICES),
        cell: z.string(),
        maxColumns: z.number().int().min(1).max(MAX_PIVOT_COLUMNS),
        /** The read spec said `maxColumns` itself, so a save keeps even the default. */
        explicitMax: z.boolean(),
        /** The field a pivot over a link heads its columns by; '' takes the first text field. */
        labelField: z.string(),
        /** A date range param whose every bucket is a column (spec pivot.range); '' : only the buckets with rows. */
        range: z.string(),
        /** The pivot columns come just before the cell column, which follows as their total (pivot.order "spec"). */
        totalAfter: z.boolean(),
    }),
    layout: z.array(layoutItemSchema).max(MAX_LAYOUT_ITEMS),
    /** A formula that labels each opened (detail) row; null: the first column's value (E7). */
    detailLabel: formulaDraftSchema.nullable(),
    /** The one-line width gate and the line layouts per width (E14), kept as the spec says them. */
    display: z.object({
        oneLine: displayOneLineSchema.optional(),
        layouts: z.array(displayLayoutSchema).max(DISPLAY_WIDTHS.length),
    }),
    /** Empty parts the read spec spelled out (`detail: {}`, `display: {}`), kept by a save. */
    explicit: z.object({ detail: z.boolean(), display: z.boolean() }),
})
export type BuilderDraft = z.infer<typeof draftShape>

type Issue = { path: (string | number)[]; message: string }

export const NEVER_CUT =
    'Numbers, amounts, percents, dates and yes/no values are never cut: only Extra room can change'

// What a figure may take (core/server/reports/fit.go fitRefusals): flexGrow, and a
// minWidth that keeps the whole value.
const isWholeMin = (minWidth: string | undefined) =>
    minWidth === undefined || minWidth === 'auto' || minWidth === 'max-content'

function fitIssues(draft: BuilderDraft, catalog: Catalog): Issue[] {
    const types = columnTypes(draft, catalog)
    return draft.columns.flatMap((column, i): Issue[] => {
        const fit = tidyFit(column.fit)
        if (!fit || !isFigureType(types.get(column.id))) return []
        const { flexGrow: _flexGrow, minWidth, ...cutting } = fit
        if (isWholeMin(minWidth) && Object.keys(cutting).length === 0) return []
        return [{ path: ['columns', i, 'fit'], message: NEVER_CUT }]
    })
}

/** The column with this fit, tidied; an empty fit is removed. */
export function columnWithFit(column: ColumnDraft, fit: ColumnFit | undefined): ColumnDraft {
    return { ...column, fit: tidyFit(fit) }
}

/** The layout with this minWidth, its fields in the engine's order (width, minWidth, tracks, cells). */
export function layoutWithMinWidth(
    layout: DisplayLayout,
    minWidth: string | undefined
): DisplayLayout {
    const { width, tracks, cells } = layout
    return minWidth ? { width, minWidth, tracks, cells } : { width, tracks, cells }
}

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
        if (key.kind === 'col' && key.col !== '' && !ids.includes(key.col)) {
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
        // bind.go pivotRange and pivot.order refusals.
        if (draft.pivot.range !== '') {
            const range = draft.params.find(param => param.id === draft.pivot.range)
            if (range?.type !== 'dateRange') {
                issues.push({ path: ['pivot', 'range'], message: 'Choose a date range' })
            } else if (draft.pivot.bucket === '') {
                issues.push({
                    path: ['pivot', 'range'],
                    message: 'Choose By: day, month, quarter, year or fiscal year',
                })
            }
        }
        if (draft.pivot.totalAfter && draft.columns[0]?.id === draft.pivot.cell) {
            issues.push({
                path: ['pivot', 'totalAfter'],
                message: 'The first column labels the rows: choose another column for the cells',
            })
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
        if (item.kind !== 'row') return
        if (item.formula === null && Object.keys(item.cells ?? {}).length === 0) {
            issues.push({
                path: ['layout', i, 'formula'],
                message: 'A computed row needs a formula or a column formula',
            })
        }
        if (item.within && !withinChoices(draft, i).includes(item.within)) {
            issues.push({
                path: ['layout', i, 'within'],
                message: 'Choose a section above this row',
            })
        }
        const within = item.within
        if (!within) return
        if (readsSubtotal(item.formula?.node, within)) {
            issues.push({ path: ['layout', i, 'formula'], message: READS_OWN_SECTION })
        }
        for (const [col, cell] of Object.entries(item.cells ?? {})) {
            if (readsSubtotal(cell.node, within)) {
                issues.push({ path: ['layout', i, 'cells', col], message: READS_OWN_SECTION })
            }
        }
    })
    return issues
}

// A level's having reads its columns and records; its fill reads the linked records
// and the groups above it (F21).
function levelIssues(draft: BuilderDraft, catalog: Catalog): Issue[] {
    const issues: Issue[] = []
    draft.groups.forEach((group, i) => {
        const level = levelScope(draft, catalog, i)
        const [having] = conditionProblems(group.having, catalog, draft.source, level)
        if (!offersHaving(draft, i) && hasHaving(draft, i))
            issues.push({ path: ['groups', i, 'having'], message: LAYOUT_HAVING })
        else if (having) issues.push({ path: ['groups', i, 'having'], message: having })
        if (!group.fill.on) return
        const [fill] = conditionProblems(
            group.fill.where,
            catalog,
            groupTarget(draft, catalog, group)?.key ?? draft.source,
            undefined,
            aboveOf(draft.groups, i, draft.columns)
        )
        if (fill) issues.push({ path: ['groups', i, 'fill'], message: fill })
    })
    return issues
}

const LAYOUT_HAVING = "A layout's sections are fixed rows: hide rows on a deeper group"
const PIVOT_DETAIL = 'A pivot report has no opened rows to label: turn the row label off'
const HIDE_EMPTY_LAST = 'The last level has no child groups to be empty of'
const HIDE_EMPTY_SECTIONS = "A layout's sections always show: hide empty groups on a deeper level"
const READS_OWN_SECTION = 'A row inside a section adds to it, so it cannot read that section'
const CELL_COLUMN =
    "A row's column formula fills a number, amount or percent column after the first, not the pivot's cell column"

/**
 * A level's "Show only groups where…" is offered unless the level is a layout's first
 * group, whose sections are fixed rows (bind.go layout).
 */
export function offersHaving(draft: Pick<BuilderDraft, 'layout'>, index: number): boolean {
    return index > 0 || draft.layout.length === 0
}

/** Why a level cannot skip rows with no key (bind.go groupsAndPivot), or undefined. */
function blankSkipRefusal(draft: Pick<BuilderDraft, 'groups'>, index: number) {
    if (index === draft.groups.length - 1)
        return 'The last level has no level below for its rows to pass to'
    if (index === 0) return 'The first level has no group above for its rows to sit under'
    if (draft.groups[index]?.bucket !== '') return 'A bucketed level has no blank key to skip'
    return undefined
}

/** "Rows with no <col>" (bind.go groupsAndPivot: not first, not last, not bucketed). */
export function offersBlankSkip(draft: Pick<BuilderDraft, 'groups'>, index: number): boolean {
    if (!draft.groups[index]) return false
    return blankSkipRefusal(draft, index) === undefined
}

/** The blank control shows where it is offered, or where it holds a skip to remove. */
export function showsBlankSkip(draft: Pick<BuilderDraft, 'groups'>, index: number): boolean {
    return offersBlankSkip(draft, index) || draft.groups[index]?.blank === 'skip'
}

type HideEmptyDraft = Pick<BuilderDraft, 'groups' | 'layout'>

/** Why a level cannot hide its empty groups (bind.go groupsAndPivot, layout), or undefined. */
function hideEmptyRefusal(draft: HideEmptyDraft, index: number) {
    if (index === draft.groups.length - 1) return HIDE_EMPTY_LAST
    if (index === 0 && draft.layout.length > 0) return HIDE_EMPTY_SECTIONS
    return undefined
}

/**
 * "Hide a group when every row in it is hidden": not on the last level, which has no
 * groups under it, nor on a layout's sections, which always show (bind.go). A
 * bucketed level is offered it, as bind accepts it there.
 */
export function offersHideEmpty(draft: HideEmptyDraft, index: number): boolean {
    return hideEmptyRefusal(draft, index) === undefined
}

/** The hide-empty control shows where it is offered, or where it is on to turn off. */
export function showsHideEmpty(draft: HideEmptyDraft, index: number): boolean {
    return offersHideEmpty(draft, index) || draft.groups[index]?.hideEmpty === true
}

type HiddenDraft = Pick<BuilderDraft, 'columns' | 'display' | 'pivot' | 'groups'>

/** Why a column cannot be hidden (bind.go hiddenColumns, columns, opening), or undefined. */
function hiddenRefusal(draft: HiddenDraft, index: number) {
    const column = draft.columns[index]
    if (!column) return undefined
    if (index === 0) return 'The first column labels the rows, so it is shown'
    if (labelsChildRows(draft.columns, index))
        return 'The first column of child rows labels the child rows, so it is shown'
    if (column.fit && Object.keys(column.fit).length > 0)
        return 'A hidden column is never shown, so it has no fit'
    if (draft.pivot.on && draft.pivot.cell === column.id)
        return "A pivot's cell column shows as the pivot's columns, so it is not hidden"
    if (draft.display.layouts.some(layout => layout.cells.some(cell => cell.col === column.id)))
        return 'A hidden column is never shown, so no layout places it'
    if (draft.groups.some(group => group.opening && column.id in group.opening.cells))
        return 'A hidden column is never shown, so its opening cell would not show'
    return undefined
}

/**
 * "Use only to group or sort" (bind.go hiddenColumns): not the first column, which
 * labels each row, nor one the display places or fits, nor the pivot's cell column,
 * nor one an opening row fills.
 */
export function offersHidden(draft: HiddenDraft, index: number): boolean {
    if (!draft.columns[index]) return false
    return hiddenRefusal(draft, index) === undefined
}

/** The hidden control shows where it is offered, or where it is on to turn off. */
export function showsHidden(draft: HiddenDraft, index: number): boolean {
    return offersHidden(draft, index) || draft.columns[index]?.hidden === true
}

/** The heading control is offered past the first column (it labels the rows), and stays on a first column that holds one, so it can be turned off. */
export function showsHeading(draft: Pick<BuilderDraft, 'columns'>, index: number): boolean {
    const column = draft.columns[index]
    if (!column) return false
    return index > 0 || Boolean(column.heading)
}

/** "Add a row to this section": the sections before the row (bind.go layout within). */
export function withinChoices(draft: Pick<BuilderDraft, 'layout'>, index: number): string[] {
    return draft.layout.slice(0, index).flatMap(item => (item.kind === 'section' ? [item.id] : []))
}

/**
 * "This column: …": number, amount and percent columns after the first (bind.go
 * layoutCells, layout.go computedLeg); the pivot's cell column shows as the pivot's
 * columns, which the row's formula fills. A Percent of a section's total is filled after
 * the statements run, so no row sets it.
 */
export function cellColumnIds(
    draft: Pick<BuilderDraft, 'columns' | 'pivot'>,
    types: ReadonlyMap<string, FieldType>
): string[] {
    const percents = sectionPercentColumns(draft.columns)
    return draft.columns.slice(1).flatMap(column => {
        const type = types.get(column.id)
        const isNumber = type !== undefined && isNumericType(type)
        const isPivotCell = draft.pivot.on && draft.pivot.cell === column.id
        return isNumber && !isPivotCell && !percents.has(column.id) ? [column.id] : []
    })
}

// bind.go groupsAndPivot: a level's blank and hideEmpty, where they are not offered.
function levelFieldIssues(draft: BuilderDraft): Issue[] {
    return draft.groups.flatMap((group, i): Issue[] => {
        const issues: Issue[] = []
        const blank = group.blank === 'skip' ? blankSkipRefusal(draft, i) : undefined
        if (blank) issues.push({ path: ['groups', i, 'blank'], message: blank })
        const hideEmpty = group.hideEmpty ? hideEmptyRefusal(draft, i) : undefined
        if (hideEmpty) issues.push({ path: ['groups', i, 'hideEmpty'], message: hideEmpty })
        return issues
    })
}

function hiddenIssues(draft: BuilderDraft): Issue[] {
    return draft.columns.flatMap((column, i): Issue[] => {
        if (!column.hidden) return []
        const message = hiddenRefusal(draft, i)
        return message ? [{ path: ['columns', i, 'hidden'], message }] : []
    })
}

// bind.go layoutCells and layout.go computedLeg: a row's cells only in the columns
// "This column: …" offers.
function cellIssues(draft: BuilderDraft, catalog: Catalog): Issue[] {
    const offered = new Set(cellColumnIds(draft, columnTypes(draft, catalog)))
    const ids = new Set(draft.columns.map(column => column.id))
    return draft.layout.flatMap((item, i): Issue[] => {
        if (item.kind !== 'row') return []
        return Object.keys(item.cells ?? {}).flatMap((col): Issue[] => {
            if (offered.has(col)) return []
            const path = ['layout', i, 'cells', col]
            if (!ids.has(col)) return [{ path, message: `The report has no column "${col}"` }]
            return [{ path, message: CELL_COLUMN }]
        })
    })
}

const hasHaving = (draft: Pick<BuilderDraft, 'groups'>, index: number) =>
    (draft.groups[index]?.having.items.length ?? 0) > 0

/** The having editor shows where it is offered, or where it holds conditions to remove. */
export function showsHaving(draft: Pick<BuilderDraft, 'layout' | 'groups'>, index: number) {
    return offersHaving(draft, index) || hasHaving(draft, index)
}

/**
 * The row label is offered unless the report pivots: a pivot has no opened rows
 * (bind.go detail.label); a label already set stays in view so it can be turned off.
 */
export function showsDetailLabel(draft: Pick<BuilderDraft, 'pivot' | 'detailLabel'>): boolean {
    return !draft.pivot.on || draft.detailLabel !== null
}

/**
 * The column whose cell names each opened row: the first column, or the column a row
 * label of exactly {col} repeats (assemble.go repeatedColumn). undefined for none.
 */
function detailLabelColumn(draft: Pick<BuilderDraft, 'columns' | 'detailLabel'>) {
    const node = draft.detailLabel?.node
    if (!node) return draft.columns[0]?.id
    const keys = Object.entries(node).filter(([, value]) => value !== undefined)
    return keys.length === 1 && keys[0]?.[0] === 'col' ? node.col : undefined
}

/** The count of nest hops on a column's path: 0 for a column of the report's own rows. */
const nestDepth = (column: ColumnDraft) =>
    column.kind === 'path' ? column.path.filter(hop => hop.mode === 'nest').length : 0

/** A nest level's first column names that level's child rows (nest.go firstLabel). */
function labelsChildRows(columns: readonly ColumnDraft[], index: number) {
    const column = columns[index]
    const depth = column ? nestDepth(column) : 0
    if (depth === 0) return false
    return columns.findIndex(other => nestDepth(other) === depth) === index
}

type DetailBlankDraft = Pick<BuilderDraft, 'groups' | 'columns' | 'detailLabel' | 'pivot'>

/** Why a column cannot be blank on opened rows (bind.go detailBlank), or undefined. */
function detailBlankRefusal(draft: DetailBlankDraft, index: number) {
    if (draft.groups.length === 0) return 'Group the report to leave a column blank on opened rows'
    if (draft.pivot.on) return 'A pivot report has no opened rows: show the column on them'
    if (draft.columns[index]?.id === detailLabelColumn(draft))
        return 'This column names opened rows: show it on them'
    if (labelsChildRows(draft.columns, index))
        return 'This column names child rows: show it on them'
    return undefined
}

/**
 * "Leave blank" on opened rows is offered on a grouped report that does not pivot, on
 * every column but one that names opened rows or a nest level's child rows.
 */
export function offersDetailBlank(draft: DetailBlankDraft, index: number): boolean {
    if (!draft.columns[index]) return false
    return detailBlankRefusal(draft, index) === undefined
}

function detailBlankIssues(draft: BuilderDraft): Issue[] {
    return draft.columns.flatMap((column, i): Issue[] => {
        if (column.detail !== 'blank') return []
        const message = detailBlankRefusal(draft, i)
        return message ? [{ path: ['columns', i, 'detail'], message }] : []
    })
}

const ROLLUP_ROWS =
    "Totals that add the rows shown cannot also show a formula: choose Recompute from all rows, or the column's total"
const ROLLUP_NESTED = 'A child row column has no totals to replace'
const ROLLUP_PIVOT = "A pivot's cell column is totalled by its pivot columns"
const OPENING_PIVOT = 'This level never opens, so it has no opening row'
const ROLLUP_GROUPED = 'A column a group is grouped by shows the group on its total rows'
const OPENING_LABEL_COLUMN = "The opening row's label fills the first column"
const OPENING_NESTED = 'A child row column has no opening cell'

// bind.go columns and groupsAndPivot: a rollup replaces a recomputed total of the
// report's own rows, which an unbucketed group's column never has.
function rollupIssues(draft: BuilderDraft): Issue[] {
    const grouped = new Set(draft.groups.filter(g => g.bucket === '').map(g => g.col))
    return draft.columns.flatMap((column, i): Issue[] => {
        if (!column.rollup) return []
        const path = ['columns', i, 'rollup']
        if (column.total === 'rows') return [{ path, message: ROLLUP_ROWS }]
        if (nestDepth(column) > 0) return [{ path, message: ROLLUP_NESTED }]
        if (grouped.has(column.id)) return [{ path, message: ROLLUP_GROUPED }]
        if (draft.pivot.on && draft.pivot.cell === column.id)
            return [{ path, message: ROLLUP_PIVOT }]
        return []
    })
}

// bind.go groupsAndPivot: an opening row shows under a level that opens, in the
// report's own-row columns, with its label in the first.
function openingIssues(draft: BuilderDraft): Issue[] {
    const first = draft.columns[0]?.id
    const byId = new Map(draft.columns.map(column => [column.id, column]))
    return draft.groups.flatMap((group, i): Issue[] => {
        if (!group.opening) return []
        const at = ['groups', i, 'opening']
        if (draft.pivot.on && i === draft.groups.length - 1)
            return [{ path: at, message: OPENING_PIVOT }]
        return Object.keys(group.opening.cells).flatMap((col): Issue[] => {
            const path = [...at, 'cells', col]
            const column = byId.get(col)
            if (!column) return [{ path, message: `The report has no column "${col}"` }]
            if (col === first) return [{ path, message: OPENING_LABEL_COLUMN }]
            if (nestDepth(column) > 0) return [{ path, message: OPENING_NESTED }]
            return []
        })
    })
}

export function draftIssues(draft: BuilderDraft, catalog: Catalog): Issue[] {
    const issues = [
        ...paramIssues(draft.params),
        ...groupOrderIssues(draft.params, catalog),
        ...columnIssues(draft, catalog),
        ...fitIssues(draft, catalog),
        ...detailBlankIssues(draft),
        ...rollupIssues(draft),
        ...openingIssues(draft),
        ...hiddenIssues(draft),
        ...levelFieldIssues(draft),
        ...cellIssues(draft, catalog),
        ...groupingIssues(draft, catalog),
    ]
    const [firstProblem] = conditionProblems(draft.filter, catalog, draft.source)
    if (firstProblem) issues.push({ path: ['filter'], message: firstProblem })
    if (draft.pivot.on && draft.detailLabel !== null)
        issues.push({ path: ['detailLabel'], message: PIVOT_DETAIL })
    const columnIds = draft.columns.map(column => column.id)
    return [...issues, ...levelIssues(draft, catalog), ...displayIssues(draft.display, columnIds)]
}

// The group pick text the engine refuses (params.go checkGroupLabel), in its order.
// Its limit counts UTF-8 bytes, as the engine's maxLabel does.
function paramIssues(params: readonly ParamDef[]): Issue[] {
    return params.flatMap((param, i): Issue[] => {
        const from = param.optionsFrom
        if (!from?.groupLabel) return []
        const path = ['params', i, 'optionsFrom', 'groupLabel']
        if (!from.group?.length) return [{ path, message: 'Group the list to name its groups' }]
        if (!from.groupLabel.includes(GROUP_LABEL_TOKEN)) {
            return [{ path, message: `Write ${GROUP_LABEL_TOKEN} where the group’s name goes` }]
        }
        if (utf8Length(from.groupLabel) > MAX_LABEL) {
            return [{ path, message: `Use at most ${MAX_LABEL} characters` }]
        }
        return []
    })
}

// The group orders the engine refuses (params.go checkGroupOrder, bind.go
// groupOrder): an order with no groups, and the field's own order of a group
// that is not a select.
function groupOrderIssues(params: readonly ParamDef[], catalog: Catalog): Issue[] {
    return params.flatMap((param, i): Issue[] => {
        const from = param.optionsFrom
        if (!from?.groupOrder) return []
        const path = ['params', i, 'optionsFrom', 'groupOrder']
        if (!from.group?.length) return [{ path, message: 'Group the list to order its groups' }]
        if (!groupOrderChoices(catalog, from).includes(from.groupOrder.by)) {
            return [{ path, message: 'Only a field with fixed choices has its own order' }]
        }
        return []
    })
}

// What the builder refuses in each display layout: the engine's checks in its
// order (core/server/reports/display.go display: a second layout for a width;
// bad tracks, no cells or more cells than a report has columns stop the
// layout's checks; each cell whose column the report lacks or another cell
// placed), then the first cell placeCells refuses (the client alone places
// cells).
function layoutIssues(
    layout: DisplayLayout,
    at: (string | number)[],
    columnIds: readonly string[]
): Issue[] {
    if (layout.tracks < 1 || layout.tracks > MAX_TRACKS)
        return [{ path: [...at, 'tracks'], message: `tracks is 1 to ${MAX_TRACKS}` }]
    if (layout.cells.length === 0)
        return [{ path: [...at, 'cells'], message: 'a layout places at least one column' }]
    if (layout.cells.length > MAX_COLUMNS)
        return [
            { path: [...at, 'cells'], message: `a layout places at most ${MAX_COLUMNS} columns` },
        ]
    const issues: Issue[] = []
    const placed = new Set<string>()
    layout.cells.forEach((cell, j) => {
        const path = [...at, 'cells', j, 'col']
        if (!columnIds.includes(cell.col))
            issues.push({ path, message: `the report has no column "${cell.col}"` })
        else if (placed.has(cell.col))
            issues.push({ path, message: 'a column is placed once per layout' })
        placed.add(cell.col)
    })
    const placement = placeCells(layout.tracks, layout.cells)
    if ('problem' in placement) {
        const { index, message } = placement.problem
        issues.push({ path: [...at, 'cells', index], message })
    }
    return issues
}

export function displayIssues(
    display: BuilderDraft['display'],
    columnIds: readonly string[]
): Issue[] {
    const widths = new Set<string>()
    return display.layouts.flatMap((layout, i): Issue[] => {
        const at = ['display', 'layouts', i]
        const again: Issue[] = widths.has(layout.width)
            ? [{ path: [...at, 'width'], message: 'one layout per width' }]
            : []
        widths.add(layout.width)
        return [...again, ...layoutIssues(layout, at, columnIds)]
    })
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
                  total: 'recompute',
                  explicitTotal: false,
                  labelField: '',
                  detail: 'show',
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
        pivot: NO_PIVOT,
        layout: [],
        detailLabel: null,
        display: { layouts: [] },
        explicit: { detail: false, display: false },
    }
}

const NO_PIVOT: BuilderDraft['pivot'] = {
    on: false,
    path: [],
    bucket: '',
    cell: '',
    maxColumns: DEFAULT_PIVOT_COLUMNS,
    explicitMax: false,
    labelField: '',
    range: '',
    totalAfter: false,
}

function columnDraft(
    column: SpecColumn,
    catalog: Catalog,
    baseKey: string,
    params: readonly ParamDef[],
    fit: ColumnFit | undefined
): ColumnDraft {
    const common = {
        id: column.id,
        aggregate: column.aggregate ?? '',
        scale: column.scale ?? null,
        format: column.format ?? '',
        total: column.total ?? 'recompute',
        explicitTotal: column.total !== undefined,
        labelField: column.labelField ?? '',
        detail: column.detail ?? 'show',
        ...(column.rollup ? { rollup: formulaDraft(column.rollup, params) } : {}),
        ...(fit ? { fit } : {}),
        ...(column.heading ? { heading: formulaDraft(column.heading, params) } : {}),
        ...(column.hidden ? { hidden: true } : {}),
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
        // With `cells` a row may have no formula; a save must not invent one.
        const cells = Object.entries(item.cells ?? {}).map(
            ([col, node]) => [col, formulaDraft(node, params)] as const
        )
        return {
            kind: 'row',
            id: item.row,
            label: item.label ?? item.row,
            formula: item.formula ? formulaDraft(item.formula, params) : null,
            ...(item.within ? { within: item.within } : {}),
            ...(cells.length > 0 ? { cells: Object.fromEntries(cells) } : {}),
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
              explicitMax: spec.pivot.maxColumns !== undefined,
              labelField: spec.pivot.labelField ?? '',
              range: spec.pivot.range ?? '',
              totalAfter: spec.pivot.order === 'spec',
          }
        : NO_PIVOT
    const params = spec.params ?? []
    const columns = spec.columns.map(column =>
        columnDraft(column, catalog, spec.source, params, spec.display?.columns?.[column.id])
    )
    const specGroups = spec.groups ?? []
    const base = { source: spec.source, columns, groups: specGroups, params }
    return {
        title: meta.title,
        description: meta.description,
        source: spec.source,
        params,
        columns,
        filter: nodeToFilter(spec.filter, catalog, spec.source),
        groups: specGroups.map((g, index) =>
            groupLevelDraft(g, {
                catalog,
                baseKey: spec.source,
                params,
                level: levelScope(base, catalog, index),
                fillSource: groupTarget(base, catalog, g)?.key ?? spec.source,
                above: aboveOf(specGroups, index, spec.columns),
            })
        ),
        sort: (spec.sort ?? []).map(s => sortDraft(s, params)),
        pivot,
        layout: (spec.layout ?? []).map(item => layoutItemDraft(item, params)),
        detailLabel: spec.detail?.label ? formulaDraft(spec.detail.label, params) : null,
        display: {
            ...(spec.display?.oneLine ? { oneLine: spec.display.oneLine } : {}),
            layouts: spec.display?.layouts ?? [],
        },
        explicit: { detail: Boolean(spec.detail), display: Boolean(spec.display) },
    }
}

function sortDraft(key: SpecSortKey, params: readonly ParamDef[]): SortDraft {
    const dir = { dir: key.dir ?? 'asc', explicitDir: key.dir !== undefined }
    if (key.formula) return { kind: 'formula', formula: formulaDraft(key.formula, params), ...dir }
    return { kind: 'col', col: key.col ?? '', ...dir }
}

interface LevelRead {
    catalog: Catalog
    baseKey: string
    params: readonly ParamDef[]
    level: LevelScope
    /** The source a fill's conditions are on: the linked records. */
    fillSource: string
    above: readonly AboveGroup[]
}

function openingFromSpec(
    opening: NonNullable<SpecGroup['opening']>,
    params: readonly ParamDef[]
): OpeningDraft {
    const cells = Object.entries(opening.cells ?? {}).map(
        ([col, node]) => [col, formulaDraft(node, params)] as const
    )
    return { label: opening.label, cells: Object.fromEntries(cells) }
}

// A fill on a group that is not by a link (refused by the server) reads against the
// base source, so its conditions show as fixed parts or name missing fields.
function groupLevelDraft(g: SpecGroup, read: LevelRead): GroupLevelDraft {
    const { catalog, params, level } = read
    return {
        id: g.id,
        col: g.col,
        bucket: g.bucket ?? '',
        subtotals: g.subtotals === true,
        open: g.open === true,
        label: g.label ? formulaDraft(g.label, params) : null,
        having: nodeToFilter(g.having, catalog, read.baseKey, level),
        fill: {
            on: g.fill !== undefined,
            where: nodeToFilter(g.fill?.where, catalog, read.fillSource, undefined, read.above),
        },
        ...(g.opening ? { opening: openingFromSpec(g.opening, params) } : {}),
        ...(g.blank ? { blank: g.blank } : {}),
        ...(g.hideEmpty ? { hideEmpty: true } : {}),
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
    const levelCtx = formulaContext(draft, catalog, { kind: 'level' })
    const headingCtx = formulaContext(draft, catalog, { kind: 'heading' })
    return draft.columns.map(column => {
        const out: SpecColumn = { id: column.id, label: column.label }
        if (column.kind === 'path') out.path = column.path
        else out.formula = specFormula(column.formula, ctx)
        if (column.aggregate !== '') out.aggregate = column.aggregate
        if (column.aggregate === 'avg') {
            out.scale = column.scale ?? avgScaleFor(types.get(column.id) ?? 'number')
        }
        if (column.format !== '' && showsFormat(column, types)) out.format = column.format
        if (column.total === 'rows' || column.explicitTotal) out.total = column.total
        if (column.labelField !== '') out.labelField = column.labelField
        if (column.detail === 'blank') out.detail = 'blank'
        // A rollup is worked out on the level rows (levels.go levelCtx.col).
        if (column.rollup) out.rollup = specFormula(column.rollup, levelCtx)
        if (column.heading) out.heading = specFormula(column.heading, headingCtx)
        if (column.hidden) out.hidden = true
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

/**
 * The source a group's column points at (a record group), or undefined: a bucketed
 * date has no record (rows.go reads a group's target only unbucketed).
 */
export function groupTarget(
    draft: TargetDraft,
    catalog: Catalog,
    group: { col: string; bucket?: string }
): CatalogSource | undefined {
    return group.bucket ? undefined : columnTarget(draft, catalog, group.col)
}

// A bucket's key as the engine writes it (rows.go bucketSQL), spelled by the parts
// of the group's first date: 2026, 2026-Q1, 2026-01. No part reads a day of the
// month, so a day starts as the first date itself, which labels the level by it.
function bucketText(bucket: string, first: SpecNode): SpecNode {
    const part = (op: string): SpecNode => ({ op, args: [first] })
    const month = part('month')
    switch (bucket) {
        case 'day':
            return first
        case 'year':
            return { op: 'concat', args: [part('year')] }
        case 'fiscalYear':
            return { op: 'concat', args: [part('fiscalYear')] }
        case 'quarter':
            return { op: 'concat', args: [part('year'), { value: '-Q' }, part('quarter')] }
    }
    const pad: SpecNode = {
        op: 'if',
        args: [{ op: 'lt', args: [month, { value: 10 }] }, { value: '0' }, { value: '' }],
    }
    return { op: 'concat', args: [part('year'), { value: '-' }, pad, month] }
}

/**
 * The formula a group's label starts as when "Formula…" is chosen: Join text of
 * what labels the level by default, the record's label field or the group's value.
 * A bucketed level has no one value of its date column (levels.go groupKey), so it
 * starts from the bucket's text, read through the column's field; a bucketed date
 * formula column, which a level row has no value of either, starts as empty text.
 */
export function labelStart(
    group: { id: string; col: string; bucket: string },
    target: CatalogSource | undefined,
    columns: readonly ColumnDraft[]
): FormulaDraft {
    const concat = (value: SpecNode): FormulaDraft => ({
        kind: 'editable',
        node: { op: 'concat', args: [value] },
    })
    if (target) {
        const field = firstTextField(target)?.key ?? 'id'
        return concat({ record: group.id, path: [{ field }] })
    }
    if (group.bucket === '') return concat({ col: group.col })
    const column = columns.find(c => c.id === group.col)
    if (column?.kind !== 'path') return concat({ value: '' })
    const first: SpecNode = { op: 'min', args: [{ path: column.path }] }
    return { kind: 'editable', node: bucketText(group.bucket, first) }
}

/**
 * What the conditions of group level `index` read (levels.go own): the columns with
 * one value on its rows (no row value, running value or percent) and the records
 * of its own group and the groups around it.
 */
export function levelScope(draft: ColumnsOf, catalog: Catalog, index: number): LevelScope {
    const offered = levelValueColumns(draft, catalog, index)
    const columns = columnChoices(draft, catalog)
        .filter(choice => offered.has(choice.key))
        .map(choice => {
            const field = {
                key: choice.key,
                label: choice.label,
                type: choice.type,
                aggregates: [],
                options: choice.options,
            }
            const target = columnTarget(draft, catalog, choice.key)
            return { key: choice.key, label: choice.label, field, target }
        })
    const groups = draft.groups.slice(0, index + 1)
    return { columns, records: recordGroups({ ...draft, groups }, catalog) }
}

/**
 * The params something in the draft still reads, plus newly asked ones (R20): a
 * filter, having or fill condition, or a formula anywhere. A param stays while
 * anything reads it, and leaves with the last part that does.
 */
export function keptParams(draft: BuilderDraft, ...added: ParamDef[]): ParamDef[] {
    const read = paramsInGroup(draft.filter, new Set<string>())
    const node = (n: SpecNode | undefined) => paramsInNode(n, read)
    for (const column of draft.columns) {
        if (column.kind === 'formula') node(column.formula.node)
        node(column.rollup?.node)
        node(column.heading?.node)
    }
    for (const group of draft.groups) {
        paramsInGroup(group.having, read)
        if (group.fill.on) paramsInGroup(group.fill.where, read)
        node(group.label?.node)
        for (const cell of Object.values(group.opening?.cells ?? {})) node(cell.node)
    }
    for (const key of draft.sort) if (key.kind === 'formula') node(key.formula.node)
    for (const item of draft.layout) {
        if (item.kind !== 'row') continue
        node(item.formula?.node)
        for (const cell of Object.values(item.cells ?? {})) node(cell.node)
    }
    node(draft.detailLabel?.node)
    // A kept param's options can be filtered by another param, which then stays too,
    // and so on (params.go optionsFrom.where).
    const byId = new Map([...draft.params, ...added].map(p => [p.id, p]))
    const pending = [...read, ...added.map(p => p.id)]
    for (let id = pending.pop(); id !== undefined; id = pending.pop()) {
        for (const next of paramsInNode(byId.get(id)?.optionsFrom?.where, new Set())) {
            if (!read.has(next)) pending.push(next)
            read.add(next)
        }
    }
    return [...draft.params.filter(p => read.has(p.id)), ...added]
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
            ? {
                  row: item.id,
                  label: item.label,
                  ...(item.formula ? { formula: specFormula(item.formula, ctx) } : {}),
                  ...(item.within ? { within: item.within } : {}),
                  ...(item.cells && Object.keys(item.cells).length > 0
                      ? { cells: specCells(item.cells, ctx) }
                      : {}),
              }
            : { section: item.id, title: item.title, key: sectionKey(item.key, keyType) }
    )
}

function specCells(cells: Record<string, FormulaDraft>, ctx: ReportFormulaContext) {
    const written = Object.entries(cells).map(([col, f]) => [col, specFormula(f, ctx)] as const)
    return Object.fromEntries(written)
}

function specSort(draft: BuilderDraft, catalog: Catalog): SpecSortKey[] {
    const ctx = formulaContext(draft, catalog, { kind: 'level' })
    return draft.sort.map(s => {
        const dir = s.dir === 'desc' || s.explicitDir ? { dir: s.dir } : {}
        return s.kind === 'col'
            ? { col: s.col, ...dir }
            : { formula: specFormula(s.formula, ctx), ...dir }
    })
}

function specOpening(opening: OpeningDraft, ctx: ReportFormulaContext) {
    if (Object.keys(opening.cells).length === 0) return { label: opening.label }
    return { label: opening.label, cells: specCells(opening.cells, ctx) }
}

function specGroups(draft: BuilderDraft, catalog: Catalog): SpecGroup[] {
    return draft.groups.map((g, index) => {
        const level = levelScope(draft, catalog, index)
        const ctx = formulaContext(draft, catalog, { kind: 'level', index })
        const having = filterToNode(g.having, catalog, draft.source, level)
        const fillSource = groupTarget(draft, catalog, g)?.key ?? draft.source
        const above = aboveOf(draft.groups, index, draft.columns)
        const where = g.fill.on
            ? filterToNode(g.fill.where, catalog, fillSource, undefined, above)
            : undefined
        return {
            id: g.id,
            col: g.col,
            ...(g.bucket !== '' ? { bucket: g.bucket } : {}),
            ...(g.subtotals ? { subtotals: true } : {}),
            ...(g.label ? { label: specFormula(g.label, ctx) } : {}),
            ...(having ? { having } : {}),
            ...(g.fill.on ? { fill: where ? { where } : {} } : {}),
            ...(g.open ? { open: true } : {}),
            ...(g.opening ? { opening: specOpening(g.opening, ctx) } : {}),
            ...(g.blank === 'skip' ? { blank: 'skip' as const } : {}),
            ...(g.hideEmpty ? { hideEmpty: true } : {}),
        }
    })
}

function specPivot(pivot: BuilderDraft['pivot']): NonNullable<ReportSpec['pivot']> {
    return {
        path: pivot.path,
        ...(pivot.bucket !== '' ? { bucket: pivot.bucket } : {}),
        cell: pivot.cell,
        // The engine shows DEFAULT_PIVOT_COLUMNS when none is said (bind.go pivot).
        ...(pivot.explicitMax || pivot.maxColumns !== DEFAULT_PIVOT_COLUMNS
            ? { maxColumns: pivot.maxColumns }
            : {}),
        ...(pivot.labelField !== '' ? { labelField: pivot.labelField } : {}),
        ...(pivot.range !== '' ? { range: pivot.range } : {}),
        ...(pivot.totalAfter ? { order: 'spec' as const } : {}),
    }
}

/** A column's fit in the engine's field order with no unset setting; undefined when nothing is set. */
export function tidyFit(fit: ColumnFit | undefined): ColumnFit | undefined {
    if (!fit) return undefined
    const tidy: ColumnFit = {
        ...(fit.width !== undefined ? { width: fit.width } : {}),
        ...(fit.minWidth !== undefined ? { minWidth: fit.minWidth } : {}),
        ...(fit.maxWidth !== undefined ? { maxWidth: fit.maxWidth } : {}),
        ...(fit.whiteSpace !== undefined ? { whiteSpace: fit.whiteSpace } : {}),
        ...(fit.lineClamp !== undefined ? { lineClamp: fit.lineClamp } : {}),
        ...(fit.textOverflow !== undefined ? { textOverflow: fit.textOverflow } : {}),
        ...(fit.flexGrow !== undefined ? { flexGrow: fit.flexGrow } : {}),
    }
    return Object.keys(tidy).length > 0 ? tidy : undefined
}

// Go writes a map's keys in byte order (not locale order).
function byteOrder(a: string, b: string): number {
    if (a < b) return -1
    return a > b ? 1 : 0
}

// The display in the engine's field order (oneLine, columns, layouts); an empty
// display the read spec spelled out is kept, any other empty one is left out.
function specDisplay(draft: BuilderDraft): ReportSpec['display'] {
    const fits = draft.columns
        .flatMap(column => {
            const fit = tidyFit(column.fit)
            return fit ? [[column.id, fit] as const] : []
        })
        .sort(([a], [b]) => byteOrder(a, b))
    const { oneLine, layouts } = draft.display
    const display = {
        ...(oneLine ? { oneLine } : {}),
        ...(fits.length > 0 ? { columns: Object.fromEntries(fits) } : {}),
        ...(layouts.length > 0 ? { layouts } : {}),
    }
    if (Object.keys(display).length > 0) return display
    return draft.explicit.display ? {} : undefined
}

// Parts are written in the spec's own order, so a save that changes nothing writes
// the spec it read.
export function specFromDraft(draft: BuilderDraft, catalog: Catalog): ReportSpec {
    const types = columnTypes(draft, catalog)
    const params = keptParams(draft).map(tidyParam)
    const spec: ReportSpec = {
        version: CURRENT_SPEC_VERSION,
        source: draft.source,
        ...(params.length > 0 ? { params } : {}),
        columns: specColumns(draft, catalog, types),
    }
    const filter = filterToNode(draft.filter, catalog, draft.source)
    if (filter) spec.filter = filter
    if (draft.sort.length > 0) spec.sort = specSort(draft, catalog)
    if (draft.groups.length > 0) spec.groups = specGroups(draft, catalog)
    if (draft.pivot.on) spec.pivot = specPivot(draft.pivot)
    if (draft.layout.length > 0) spec.layout = specLayout(draft, catalog, types)
    if (draft.detailLabel) {
        const ctx = formulaContext(draft, catalog, { kind: 'detail' })
        spec.detail = { label: specFormula(draft.detailLabel, ctx) }
    } else if (draft.explicit.detail) spec.detail = {}
    const display = specDisplay(draft)
    if (display) spec.display = display
    return spec
}
