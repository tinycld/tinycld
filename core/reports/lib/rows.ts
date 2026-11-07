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

function rowCells(row: ResultRow, withSubtotal: Set<string>) {
    if (row.kind === 'group' && withSubtotal.has(row.id)) return NO_CELLS
    if (row.kind === 'detail') return unlabelledCells(row)
    return row.cells
}

// A figure is never shown twice (the report shell's rule): a group row that its own
// subtotal row follows shows no figures (a closed group with no subtotal row keeps
// its figures), and a detail row shows its label once, not again in a cell.
export function visibleRows(rows: readonly ResultRow[]): VisibleRow[] {
    const withSubtotal = new Set(
        rows.flatMap(row => (row.kind === 'subtotal' && row.section ? [row.section] : []))
    )
    return rows.map(row => {
        const isGroup = row.kind === 'group'
        return {
            row,
            cells: rowCells(row, withSubtotal),
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

function uniqueNotices(notices: readonly Notice[]): Notice[] {
    const seen = new Set<string>()
    return notices.filter(notice => {
        if (seen.has(notice.code)) return false
        seen.add(notice.code)
        return true
    })
}

const EMPTY_RESULT: RunResult = {
    columns: [],
    rows: [],
    notices: [],
    topics: [],
    page: null,
    params: [],
}

const isWholeReportRow = (row: ResultRow) => row.kind === 'total' || row.kind === 'computed'

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
