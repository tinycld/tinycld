import { CARET_ROOM, INDENT } from '../components/report-table-layout'
import { shortDay } from './dates'
import { cellDisplay, headingText, type NumberFormat, rowLabelText } from './format'
import type { VisibleRow } from './rows'
import type { SpecColumn } from './spec'
import { longestWordWidth, type TextStyle, textWidth } from './text-width'
import type { FieldType, ResultColumn, RowKind } from './types'

// Review §7.1-1: every column's width comes from its content — its heading, the
// rows of the run as it first lands (group, subtotal and Total rows included) and
// fixed worst cases — never from the screen. Pure, so web, native and an export at
// a page width size columns the same way. The measure is kept for the run (D1).

/** A column's widths: what its content needs, the least it may get, and its share of spare room. */
export type ColumnMeasure = { natural: number; min: number; grow: number }
export type ColumnMeasures = Record<string, ColumnMeasure>

/** A cell's px-3 on both sides. */
export const CELL_PADDING = 24
/** A sortable heading's icon (12) and its gap-1. */
export const SORT_ICON_ROOM = 16
export const TEXT_MIN_CHARS = 8
/** Rows read for the measure: enough for a first page, bounded for a huge one. */
export const MEASURE_ROW_LIMIT = 500

const CELL: TextStyle = { size: 'sm', weight: 'normal' }
const CELL_BOLD: TextStyle = { size: 'sm', weight: 'semibold' }
const HEADING: TextStyle = { size: 'xs', weight: 'semibold', uppercase: true }

/** The least a text column is held to: eight average letters. */
export const TEXT_MIN_WIDTH = textWidth('n'.repeat(TEXT_MIN_CHARS), CELL)

const BOLD_KINDS: ReadonlySet<RowKind> = new Set(['group', 'subtotal', 'computed', 'total'])

/** A row's text style, as ReportRow's ROW_WEIGHT draws it. */
export function cellTextStyle(kind: RowKind): TextStyle {
    return BOLD_KINDS.has(kind) ? CELL_BOLD : CELL
}

// Aligned or fixed-length values an ellipsis would ruin: they keep their width (R12).
const FIGURE_TYPES: ReadonlySet<FieldType> = new Set([
    'number',
    'money',
    'percent',
    'date',
    'datetime',
    'bool',
])

export function isFigureColumn(column: ResultColumn): boolean {
    return (
        FIGURE_TYPES.has(column.type) || (!!column.totalType && FIGURE_TYPES.has(column.totalType))
    )
}

// The widest value a column of this type may show on a later page, so a column
// sized by a first page of small values still holds the usual ones. Amounts and
// numbers are formatted as the column draws them (currency, ÷1000, cents, a minus
// or parentheses), so the worst case follows the reader's number format.
const WORST_DAY = shortDay('2026-09-30')
const WORST_TEXT: Partial<Record<FieldType, string>> = {
    date: WORST_DAY,
    datetime: WORST_DAY,
    bool: 'Yes',
    percent: '-100.0%',
}
/** −99,999.99 in cents, and −99,999. */
const WORST_VALUE: Partial<Record<FieldType, number>> = { money: -9_999_999, number: -99_999 }

function worstText(
    column: ResultColumn,
    item: VisibleRow,
    format: NumberFormat
): string | undefined {
    const value = WORST_VALUE[column.type]
    if (value === undefined) return WORST_TEXT[column.type]
    return cellDisplay(column, item.row, value, format).text
}

/**
 * How deep a report's rows go: its group levels and its levels of nested child
 * rows. `pivot`: the last group level holds the pivot's cells, so its groups never
 * open (assemble.go canOpen) and no row comes under them.
 */
export type RowLevels = { groups: number; nests: number; pivot?: boolean }
/** A report with no groups and no child rows. */
export const FLAT_LEVELS: RowLevels = { groups: 0, nests: 0 }

/**
 * A report's row levels, from its spec: one per group level, and one per `nest`
 * hop of its deepest column (the engine nests one chain of child rows). The spec,
 * not the loaded rows, so the layout does not change as groups open or pages load.
 */
export function rowLevelsOf(
    spec: {
        groups?: readonly unknown[] | null
        columns: readonly Pick<SpecColumn, 'path'>[]
        pivot?: unknown
    } | null
): RowLevels {
    if (!spec) return FLAT_LEVELS
    const nestsOf = (column: Pick<SpecColumn, 'path'>) =>
        (column.path ?? []).filter(hop => hop.mode === 'nest').length
    const levels = {
        groups: spec.groups?.length ?? 0,
        nests: Math.max(0, ...spec.columns.map(nestsOf)),
    }
    return spec.pivot ? { ...levels, pivot: true } : levels
}

// The left edge of a group row's label at a level (1 is the outermost): INDENT per
// level above it, then its caret when the group can open (ReportRow labelStyle).
const groupLabelStart = (level: number, canOpen: boolean) =>
    (level - 1) * INDENT + (canOpen ? CARET_ROOM : 0)

// The deepest label's left edge. A detail row sits at the group levels, a child
// row one more per level. In a pivot report the last level's groups never open:
// they draw no caret and hold no detail or child row.
export function deepestLabelStart({ groups, nests, pivot }: RowLevels): number {
    if (groups === 0) return nests * INDENT
    if (pivot) {
        const pivotRows = groupLabelStart(groups, false)
        return groups > 1 ? Math.max(pivotRows, groupLabelStart(groups - 1, true)) : pivotRows
    }
    return Math.max((groups + nests) * INDENT, groupLabelStart(groups, true))
}

/** The label column's room besides its text: the base indent, the deepest label's start and pr-3. */
export function labelChrome(levels: RowLevels): number {
    return INDENT + deepestLabelStart(levels) + CELL_PADDING / 2
}

/** The width a row's whole label needs, as RowLabel draws it (ReportRow labelStyle). */
export function labelNeed(item: VisibleRow, format: NumberFormat): number {
    const { row } = item
    const start = INDENT + row.depth * INDENT + (item.isToggle ? CARET_ROOM : 0)
    return start + textWidth(rowLabelText(row, format), cellTextStyle(row.kind)) + CELL_PADDING / 2
}

type Widths = { natural: number; min: number }

function headingWidths(column: ResultColumn, format: NumberFormat): Widths {
    const text = headingText(column, format)
    const icon = column.sortable ? SORT_ICON_ROOM : 0
    return {
        natural: textWidth(text, HEADING) + icon + CELL_PADDING,
        min: longestWordWidth(text, HEADING) + icon + CELL_PADDING,
    }
}

const hasValue = (value: unknown) => value !== undefined && value !== null

// The widest shown value of a column, with its type's worst case when it has any
// value; 0 when every cell is blank (D3: it is then as wide as its heading).
function widestCell(column: ResultColumn, rows: readonly VisibleRow[], format: NumberFormat) {
    let widest = 0
    let valued: VisibleRow | undefined
    for (const item of rows) {
        const text = cellDisplay(column, item.row, item.cells[column.id], format).text
        if (!text) continue
        valued = valued ?? item
        widest = Math.max(widest, textWidth(text, cellTextStyle(item.row.kind)))
    }
    const worst = valued ? worstText(column, valued, format) : undefined
    if (worst) widest = Math.max(widest, textWidth(worst, CELL))
    return widest === 0 ? 0 : widest + CELL_PADDING
}

// A non-detail row spans the empty cells after its label (Task 6), so its label
// sizes the label column only when the first other cell has a value.
function spansNothing(item: VisibleRow, others: readonly ResultColumn[]) {
    const [first] = others
    return first !== undefined && hasValue(item.cells[first.id])
}

const TYPED_LABELS: ReadonlySet<FieldType> = new Set(['date', 'datetime', 'number', 'money'])

function labelWidths(
    others: readonly ResultColumn[],
    rows: readonly VisibleRow[],
    format: NumberFormat
): { widest: number; isTyped: boolean } {
    let widest = 0
    let isTyped = false
    for (const item of rows) {
        const { row } = item
        if (row.kind !== 'detail' && !spansNothing(item, others)) continue
        widest = Math.max(widest, labelNeed(item, format))
        if (row.kind !== 'detail' || !row.labelType || !TYPED_LABELS.has(row.labelType)) continue
        isTyped = true
        if (row.labelType === 'date' || row.labelType === 'datetime') {
            const start = INDENT + row.depth * INDENT
            widest = Math.max(widest, start + textWidth(WORST_DAY, CELL) + CELL_PADDING / 2)
        }
    }
    return { widest, isTyped }
}

/**
 * What the run's measure depends on (D1): the run (design, params, sort), the
 * number format, the shown columns and the row levels. A later page, a fold and a
 * live re-run keep the key, so the columns never move under the reader.
 */
export function measureKey(input: {
    runKey: string
    format: NumberFormat
    columns: readonly ResultColumn[]
    levels: RowLevels
}): string {
    const { runKey, format, columns, levels } = input
    const shape = columns.map(column => [column.id, column.label, column.sortable === true])
    return JSON.stringify([runKey, format, shape, levels])
}

export function measureColumns(input: {
    columns: readonly ResultColumn[]
    rows: readonly VisibleRow[]
    format: NumberFormat
    levels: RowLevels
}): ColumnMeasures {
    const { columns, format, levels } = input
    const rows = input.rows.slice(0, MEASURE_ROW_LIMIT)
    const [label, ...others] = columns
    const measures: ColumnMeasures = {}
    if (label) {
        const heading = headingWidths(label, format)
        const { widest, isTyped } = labelWidths(others, rows, format)
        const natural = Math.max(heading.natural, widest)
        const floor = isTyped ? widest : Math.min(natural, labelChrome(levels) + TEXT_MIN_WIDTH)
        measures[label.id] = { natural, min: Math.max(heading.min, floor), grow: 1 }
    }
    for (const column of others) {
        const heading = headingWidths(column, format)
        const widest = widestCell(column, rows, format)
        const natural = Math.max(heading.natural, widest)
        if (isFigureColumn(column)) {
            measures[column.id] = { natural, min: Math.max(heading.min, widest), grow: 0 }
            continue
        }
        const floor = Math.min(natural, TEXT_MIN_WIDTH + CELL_PADDING)
        measures[column.id] = { natural, min: Math.max(heading.min, floor), grow: 1 }
    }
    return measures
}
