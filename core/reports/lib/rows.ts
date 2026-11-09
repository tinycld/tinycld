import type { CellValue, Notice, ResultRow, RunResult } from './types'

// The rows of a run as the table draws them. Groups load lazily on the server
// (options.expand), so a closed group's children are simply not in the result:
// folding is a new run, never a client-side hide.

export type VisibleRow = {
    row: ResultRow
    cells: Record<string, CellValue>
    /** A group row that can open or close. */
    isToggle: boolean
    isOpen: boolean
}

const NO_CELLS: Record<string, CellValue> = {}

// The engine names the column whose cell a detail row's label repeats
// (labelColumn): the first column, a child row's level's first column, or the
// column a detail label of exactly {col} reads. The label already shows it, so
// that cell is left out. A label that repeats no cell (any other detail label
// formula) leaves every cell.
function unlabelledCells(row: ResultRow) {
    const { labelColumn } = row
    if (!labelColumn) return row.cells
    return Object.fromEntries(Object.entries(row.cells).filter(([id]) => id !== labelColumn))
}

/** What visibleRows reads of the spec: which group rows end in a subtotal row. */
export type SubtotalSpec = {
    groups?: readonly { subtotals?: boolean }[] | null
    layout?: readonly unknown[] | null
}

// Whether the engine ends this group row with its own subtotal row
// (assemble.go groupWithLabel): an open group of a level with subtotals; in a
// layout report every group of such a level, open or closed, and every layout
// section (level 1). A paged run may send that subtotal row on a later page.
function endsInSubtotal(row: ResultRow, spec: SubtotalSpec): boolean {
    const isLayout = (spec.layout?.length ?? 0) > 0
    // With a skipped level (groups[].blank) a row's depth no longer gives its level.
    const level = row.level ?? row.depth + 1
    const hasSubtotals = (isLayout && level === 1) || spec.groups?.[level - 1]?.subtotals === true
    return hasSubtotals && (row.expanded === true || isLayout)
}

function rowCells(row: ResultRow, withSubtotal: Set<string>, spec: SubtotalSpec | null) {
    if (row.kind === 'group' && withSubtotal.has(row.id)) return NO_CELLS
    if (row.kind === 'group' && spec && endsInSubtotal(row, spec)) return NO_CELLS
    if (row.kind === 'detail') return unlabelledCells(row)
    return row.cells
}

// A figure is never shown twice (the report shell's rule): a group row that its own
// subtotal row follows shows no figures (a closed group with no subtotal row keeps
// its figures), and a detail row shows its label once, not again in a cell. With
// the spec, a group row whose subtotal row has not loaded yet (it comes on a later
// page) shows no figures either, so its figures never show and then go.
export function visibleRows(
    rows: readonly ResultRow[],
    spec: SubtotalSpec | null = null
): VisibleRow[] {
    const withSubtotal = new Set(
        rows.flatMap(row => (row.kind === 'subtotal' && row.section ? [row.section] : []))
    )
    // The server never sends such a row under a folded section; this keeps a stale
    // page (held or merged from before a fold) from showing one.
    const folded = new Set(
        rows.flatMap(row => (row.kind === 'group' && row.expanded !== true ? [row.id] : []))
    )
    const shown = rows.filter(row => !(isSectionRow(row) && row.section && folded.has(row.section)))
    return shown.map(row => {
        const isGroup = row.kind === 'group'
        return {
            row,
            cells: rowCells(row, withSubtotal, spec),
            isToggle: isGroup && row.canExpand === true,
            isOpen: isGroup && row.expanded === true,
        }
    })
}

/**
 * Which groups a run opens: every one, those whose keys are listed, or — on the
 * first run, before any press — the levels the report marks open (the engine's
 * default, sent as no expand at all).
 */
export type Expansion = { all: boolean; keys: readonly unknown[][]; isDefault?: boolean }

export const DEFAULT_EXPANSION: Expansion = { all: false, keys: [], isDefault: true }
export const NO_EXPANSION: Expansion = { all: false, keys: [] }
export const EXPAND_ALL: Expansion = { all: true, keys: [] }

const keyText = (key: readonly unknown[] | undefined) => JSON.stringify(key ?? [])

function openGroupKeys(rows: readonly ResultRow[]): unknown[][] {
    return rows.filter(row => row.kind === 'group' && row.expanded).map(row => row.groupKey ?? [])
}

// The row and every group row whose chain of sections reaches it. Rows come parent
// first, so one pass finds them all.
function groupAndDescendants(row: ResultRow, rows: readonly ResultRow[]): Set<string> {
    const ids = new Set([row.id])
    for (const r of rows) {
        if (r.kind === 'group' && r.section !== undefined && ids.has(r.section)) ids.add(r.id)
    }
    return ids
}

export function toggleGroup(
    expansion: Expansion,
    row: ResultRow,
    rows: readonly ResultRow[]
): Expansion {
    // Under Expand all, or the levels the report marks open, the open groups are the
    // ones the result shows open.
    const current = expansion.all || expansion.isDefault ? openGroupKeys(rows) : [...expansion.keys]
    const key = row.groupKey ?? []
    if (!row.expanded) {
        const text = keyText(key)
        return {
            all: false,
            keys: current.some(k => keyText(k) === text) ? current : [...current, key],
        }
    }
    const closing = groupAndDescendants(row, rows)
    const closedKeys = new Set(rows.filter(r => closing.has(r.id)).map(r => keyText(r.groupKey)))
    return { all: false, keys: current.filter(k => !closedKeys.has(keyText(k))) }
}

export function foldAllLabel(rows: readonly ResultRow[]): 'Collapse all' | 'Expand all' | null {
    const toggles = rows.filter(row => row.kind === 'group' && row.canExpand)
    if (toggles.length === 0) return null
    return toggles.some(row => row.expanded) ? 'Collapse all' : 'Expand all'
}

export function foldAllTarget(rows: readonly ResultRow[]): Expansion {
    return foldAllLabel(rows) === 'Collapse all' ? NO_EXPANSION : EXPAND_ALL
}

export function expansionOptions(expansion: Expansion): {
    expand?: unknown[][]
    expandAll?: boolean
} {
    if (expansion.isDefault) return {}
    if (expansion.all) return { expandAll: true }
    // An explicit empty list opens nothing, groups marked open included.
    return { expand: [...expansion.keys] }
}

/** A result with no detail row and no group row is empty: totals alone do not count. */
export function hasRows(rows: readonly ResultRow[]): boolean {
    return rows.some(row => row.kind === 'detail' || row.kind === 'group')
}

const union = (a: readonly string[] = [], b: readonly string[] = []) => [...new Set([...a, ...b])]

// A notice's sentences as the server sent them: its parts, or its message as one.
const sentencesOf = (notice: Notice): string[] => notice.parts ?? [notice.message]

// A page names only what its own rows and conditions show (a mixedCurrency
// notice names the columns blank on that page), so one code's notices from
// several pages join: their columns, conditions and sentences each once, in
// the order the pages first name them. A sentence is one of the server's
// parts (or a whole message of one sentence), never cut from the text here.
function joinNotices(a: Notice, b: Notice): Notice {
    const parts = union(sentencesOf(a), sentencesOf(b))
    const notice: Notice = { ...a, message: parts.join(' ') }
    if (parts.length > 1) notice.parts = parts
    if (a.columns || b.columns) notice.columns = union(a.columns, b.columns)
    if (a.conditions || b.conditions) notice.conditions = union(a.conditions, b.conditions)
    return notice
}

function uniqueNotices(notices: readonly Notice[]): Notice[] {
    const byCode = new Map<string, Notice>()
    for (const notice of notices) {
        const earlier = byCode.get(notice.code)
        byCode.set(notice.code, earlier ? joinNotices(earlier, notice) : notice)
    }
    return [...byCode.values()]
}

const EMPTY_RESULT: RunResult = {
    columns: [],
    rows: [],
    notices: [],
    topics: [],
    page: null,
    params: [],
}

// A computed row inside a section (layout[].within) belongs to that section: it
// is neither a whole-report row nor shown under a folded section.
const isSectionRow = (row: ResultRow) => row.kind === 'computed' && row.section !== undefined

const isWholeReportRow = (row: ResultRow) =>
    row.kind === 'total' || (row.kind === 'computed' && !isSectionRow(row))

// The rows of an earlier page without the whole-report rows the server appends
// after the page's slice: those repeat on every page, and only the newest page's
// figures are current.
function withoutAppendedTotals(rows: readonly ResultRow[]): readonly ResultRow[] {
    let end = rows.length
    while (end > 0 && isWholeReportRow(rows[end - 1])) end--
    return rows.slice(0, end)
}

// Every report pages by keyset (R9). A page holds its detail rows and, in a grouped
// report, the group, opening and subtotal rows between them in document order. A
// total or computed row inside the page's slice keeps its place there (a Total per
// currency block, a layout's computed rows); the whole-report rows after the slice's
// end are appended to every page. Pages are joined in order: each earlier page's
// trailing run of total and computed rows goes, and a row whose id an earlier page
// gave is kept once (R10), so a seam never shows a section's row twice.
export function mergePages(pages: readonly RunResult[]): RunResult {
    const last = pages[pages.length - 1]
    if (!last) return EMPTY_RESULT
    const seen = new Set<string>()
    const rows: ResultRow[] = []
    for (const page of pages) {
        const pageRows = page === last ? page.rows : withoutAppendedTotals(page.rows)
        for (const row of pageRows) {
            if (seen.has(row.id)) continue
            seen.add(row.id)
            rows.push(row)
        }
    }
    return {
        ...last,
        rows,
        notices: uniqueNotices(pages.flatMap(page => page.notices)),
    }
}

/** The merged run as the list shows it, and the whole-report rows it holds back. */
export type ShownResult = { result: RunResult; held: readonly ResultRow[] }

const NO_HELD: readonly ResultRow[] = []

// D8 (H1): every page ends with the whole-report rows (the Total, a layout's
// computed rows). While a later page is left, the list must not end in them, or
// the reader sees an end that is not the end; they are held back until the last
// page lands. The column measure still reads them (useTableLayout), so the Total's
// width is known from the first page. Not done in mergePages: runFirstPage joins
// several server pages into one first page with it, and must keep the Total.
export function holdWholeReportRows(result: RunResult): ShownResult {
    if (!result.page?.next) return { result, held: NO_HELD }
    const rows = withoutAppendedTotals(result.rows)
    if (rows.length === result.rows.length) return { result, held: NO_HELD }
    return { result: { ...result, rows: [...rows] }, held: result.rows.slice(rows.length) }
}
