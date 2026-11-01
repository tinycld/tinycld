import type { CellValue, Notice, ResultColumn, ResultRow, RunResult } from './types'

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

// The engine labels a nested child row with its nest level's first column: the first
// result column the row has a cell for. That cell would repeat the label, so it is
// left out. Every result column counts, hidden or not, as it does for the engine.
function childCells(row: ResultRow, columns: readonly ResultColumn[]) {
    const labelColumn = columns.find(column => column.id in row.cells)
    if (!labelColumn) return row.cells
    return Object.fromEntries(Object.entries(row.cells).filter(([id]) => id !== labelColumn.id))
}

function rowCells(row: ResultRow, columns: readonly ResultColumn[], withSubtotal: Set<string>) {
    if (row.kind === 'group' && withSubtotal.has(row.id)) return NO_CELLS
    if (row.parent !== undefined) return childCells(row, columns)
    return row.cells
}

// A figure is never shown twice (the report shell's rule): a group row that its own
// subtotal row follows shows no figures (a closed group with no subtotal row keeps
// its figures), and a child row shows its label once, not again in a cell.
export function visibleRows(
    rows: readonly ResultRow[],
    columns: readonly ResultColumn[]
): VisibleRow[] {
    const withSubtotal = new Set(
        rows.flatMap(row => (row.kind === 'subtotal' && row.section ? [row.section] : []))
    )
    return rows.map(row => {
        const isGroup = row.kind === 'group'
        return {
            row,
            cells: rowCells(row, columns, withSubtotal),
            isToggle: isGroup && row.canExpand === true,
            isOpen: isGroup && row.expanded === true,
        }
    })
}

/** Which groups a run opens: every one, or those whose group keys are listed. */
export type Expansion = { all: boolean; keys: readonly unknown[][] }

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
    // Under Expand all, the open groups are the ones the result shows open.
    const current = expansion.all ? openGroupKeys(rows) : [...expansion.keys]
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
    if (expansion.all) return { expandAll: true }
    return expansion.keys.length > 0 ? { expand: [...expansion.keys] } : {}
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

// Only ungrouped reports page (core/server/reports); a grouped run is one page whose
// rows are in tree order. Each page repeats the whole-report rows (total, computed),
// which follow its detail rows, and only the newest page's figures are current, so the
// earlier pages' detail rows go before the newest page's rows, kept as sent.
export function mergePages(pages: readonly RunResult[]): RunResult {
    const last = pages[pages.length - 1]
    if (!last) return EMPTY_RESULT
    return {
        ...last,
        rows: [
            ...pages.slice(0, -1).flatMap(page => page.rows.filter(row => row.kind === 'detail')),
            ...last.rows,
        ],
        notices: uniqueNotices(pages.flatMap(page => page.notices)),
    }
}
