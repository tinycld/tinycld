import { CARET_ROOM, INDENT } from '../components/report-table-layout'
import { shortDay } from './dates'
import { cellDisplay, headingText, type NumberFormat, rowLabelText } from './format'
import type { VisibleRow } from './rows'
import { type ColumnFit, type FitLength, parseFitLength, type SpecColumn } from './spec'
import { longestWordWidth, type TextStyle, textWidth } from './text-width'
import type { FieldType, ResultColumn, RowKind } from './types'

// Review §7.1-1: every column's width comes from its content — its heading, the
// rows of the run as it first lands (group, subtotal and Total rows included) and
// fixed worst cases — and its fit settings (display.columns), never from the
// screen. Pure, so web, native and an export at a page width size columns the same
// way. The measure is kept for the run (D1).

/**
 * A column's widths: what its content needs, the least it may get, its share of
 * spare room; and from its fit (display.columns) the most it may get, the lines its
 * text may wrap to, and the parts of the line width a % setting asks for (0–1),
 * which the fit resolves at each line's width (table-lines.ts atWidth).
 */
export type ColumnMeasure = {
    natural: number
    min: number
    grow: number
    max?: number
    lines?: number
    pct?: { width?: number; min?: number; max?: number }
}
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
/** The width of "0" in a cell: a ch. */
export const CH_WIDTH = textWidth('0', CELL)
/** The lines a wrapping column's text may take when its fit gives no lineClamp. */
export const DEFAULT_LINE_CLAMP = 2

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

export function isFigureType(type: FieldType | undefined): boolean {
    return type !== undefined && FIGURE_TYPES.has(type)
}

export function isFigureColumn(column: ResultColumn): boolean {
    return isFigureType(column.type) || isFigureType(column.totalType)
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
    let longestWord = 0
    let valued: VisibleRow | undefined
    for (const item of rows) {
        const text = cellDisplay(column, item.row, item.cells[column.id], format).text
        if (!text) continue
        valued = valued ?? item
        const style = cellTextStyle(item.row.kind)
        widest = Math.max(widest, textWidth(text, style))
        longestWord = Math.max(longestWord, longestWordWidth(text, style))
    }
    const worst = valued ? worstText(column, valued, format) : undefined
    if (worst) widest = Math.max(widest, textWidth(worst, CELL))
    return {
        widest: widest === 0 ? 0 : widest + CELL_PADDING,
        longestWord: longestWord === 0 ? 0 : longestWord + CELL_PADDING,
    }
}

// A non-detail row spans the empty cells after its label (Task 6), so its label
// sizes the label column only when the first other cell has a value.
function spansNothing(item: VisibleRow, others: readonly ResultColumn[]) {
    const [first] = others
    return first !== undefined && hasValue(item.cells[first.id])
}

const TYPED_LABELS: ReadonlySet<FieldType> = new Set(['date', 'datetime', 'number', 'money'])

// The width a row's label needs for its longest word, as labelNeed for the whole label.
function labelWordNeed(item: VisibleRow, format: NumberFormat): number {
    const { row } = item
    const start = INDENT + row.depth * INDENT + (item.isToggle ? CARET_ROOM : 0)
    const word = longestWordWidth(rowLabelText(row, format), cellTextStyle(row.kind))
    return start + word + CELL_PADDING / 2
}

function labelWidths(
    others: readonly ResultColumn[],
    rows: readonly VisibleRow[],
    format: NumberFormat
): { widest: number; longestWord: number; isTyped: boolean } {
    let widest = 0
    let longestWord = 0
    let isTyped = false
    for (const item of rows) {
        const { row } = item
        if (row.kind !== 'detail' && !spansNothing(item, others)) continue
        widest = Math.max(widest, labelNeed(item, format))
        longestWord = Math.max(longestWord, labelWordNeed(item, format))
        if (row.kind !== 'detail' || !row.labelType || !TYPED_LABELS.has(row.labelType)) continue
        isTyped = true
        if (row.labelType === 'date' || row.labelType === 'datetime') {
            const start = INDENT + row.depth * INDENT
            widest = Math.max(widest, start + textWidth(WORST_DAY, CELL) + CELL_PADDING / 2)
        }
    }
    return { widest, longestWord, isTyped }
}

// What a column's content asks for before its fit. Widths include the padding
// (`pad`: a cell's px-3 on both sides, or the label's chrome); `floor` is the
// automatic least width (eight letters, or the whole value of a typed label).
type Content = {
    natural: number
    headingMin: number
    floor: number
    minContent: number
    pad: number
}

type Pct = NonNullable<ColumnMeasure['pct']>

const autoMin = (c: Content) => Math.max(c.headingMin, Math.min(c.natural, c.floor))
const pctOf = (length: FitLength) => length.value / 100

// A ch or px length is the text's width, so the padding is added; % is resolved later.
function lengthPx(length: FitLength, pad: number): number {
    const text = length.unit === 'ch' ? length.value * CH_WIDTH : length.value
    return Math.ceil(text) + pad
}

function applyWidth(out: ColumnMeasure, c: Content, value: string | undefined, pct: Pct) {
    const length = value ? parseFitLength(value) : null
    if (!length) return
    if (length.unit === '%') {
        pct.width = pctOf(length)
        return
    }
    const px = Math.max(c.headingMin, lengthPx(length, c.pad))
    out.natural = px
    out.min = px
    out.max = px
}

function applyMin(out: ColumnMeasure, c: Content, value: string | undefined, pct: Pct) {
    if (value === 'min-content') out.min = Math.max(c.headingMin, c.minContent)
    else if (value === 'max-content') out.min = c.natural
    else if (value && value !== 'auto') {
        const length = parseFitLength(value)
        if (length?.unit === '%') pct.min = pctOf(length)
        else if (length) out.min = Math.max(c.headingMin, lengthPx(length, c.pad))
    }
    out.natural = Math.max(out.natural, out.min)
    if (out.max !== undefined) out.max = Math.max(out.max, out.min)
}

function applyMax(out: ColumnMeasure, c: Content, value: string | undefined, pct: Pct) {
    const length = value && value !== 'none' ? parseFitLength(value) : null
    if (!length) return
    if (length.unit === '%') {
        pct.max = pctOf(length)
        return
    }
    const cap = Math.max(out.min, lengthPx(length, c.pad))
    out.max = out.max === undefined ? cap : Math.min(out.max, cap)
    out.natural = Math.min(out.natural, out.max)
}

/**
 * Room added to a wrapping column's longest word: text-width.ts estimates widths
 * from a per-letter table, and a word a px wider than its line is split on native.
 */
export const WORD_MARGIN = 2

// A wrapping column never goes under its longest word, whatever its width or
// minWidth: native text splits a word wider than its line. Its natural and max
// follow, so a % setting (resolved from `min` in table-lines atWidth) keeps it too.
// A one-word value's natural width grows by WORD_MARGIN with it: that is intended.
function holdLongestWord(out: ColumnMeasure, c: Content) {
    if (c.minContent === 0) return
    out.min = Math.max(out.min, Math.min(c.natural, c.minContent) + WORD_MARGIN)
    out.natural = Math.max(out.natural, out.min)
    if (out.max !== undefined) out.max = Math.max(out.max, out.min)
}

// A text column's measure under its fit, read as CSS (design §3.2): `width` sets
// it, `minWidth` raises its least width (never under the heading's longest word),
// `maxWidth` caps it but never under its least. A wrap may narrow it to its
// longest word (or eight letters), never cutting a word: after width, minWidth and
// maxWidth, a wrap is raised to its longest word (holdLongestWord).
function fitted(c: Content, fit: ColumnFit | undefined, grow: number): ColumnMeasure {
    const out: ColumnMeasure = { natural: c.natural, min: autoMin(c), grow: fit?.flexGrow ?? grow }
    if (!fit) return out
    const pct: Pct = {}
    const wraps = fit.whiteSpace === 'normal'
    if (wraps) {
        out.lines = fit.lineClamp ?? DEFAULT_LINE_CLAMP
        out.min = Math.max(c.headingMin, Math.min(c.natural, Math.max(c.minContent, c.floor)))
    }
    applyWidth(out, c, fit.width, pct)
    applyMin(out, c, fit.minWidth, pct)
    applyMax(out, c, fit.maxWidth, pct)
    if (wraps) holdLongestWord(out, c)
    if (Object.keys(pct).length > 0) out.pct = pct
    return out
}

// A figure is never cut (decision 2): its fit gives only its share of spare room.
const wholeMeasure = (c: Content, fit: ColumnFit | undefined, grow: number): ColumnMeasure => ({
    natural: c.natural,
    min: autoMin(c),
    grow: fit?.flexGrow ?? grow,
})

const NO_FITS: Readonly<Record<string, ColumnFit>> = {}

/**
 * A column's share of spare room when its fit sets no flexGrow (design §3.1): the
 * label column and text take 1, other figures 0. The builder reads it too, so its
 * picker shows the share the measure gives.
 */
export function defaultFlexGrow({ isLabel, isFigure }: { isLabel: boolean; isFigure: boolean }) {
    return isLabel || !isFigure ? 1 : 0
}

// A pivot's cell columns (`amount_q1`) take the fit of their spec column
// (`amount`): display.columns is keyed by spec column.
const fitOf = (fits: Readonly<Record<string, ColumnFit>>, column: ResultColumn) =>
    fits[column.pivot?.cell ?? column.id]

// The heading is part of the measure's key, so a heading that changes width
// (another day) measures the column again.
const headingKey = (column: ResultColumn) => (column.heading ? String(column.heading.value) : '')

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
    fits?: Readonly<Record<string, ColumnFit>>
}): string {
    const { runKey, format, columns, levels } = input
    const shape = columns.map(column => [
        column.id,
        column.label,
        headingKey(column),
        column.sortable === true,
    ])
    return JSON.stringify([runKey, format, shape, levels, input.fits ?? NO_FITS])
}

export function measureColumns(input: {
    columns: readonly ResultColumn[]
    rows: readonly VisibleRow[]
    format: NumberFormat
    levels: RowLevels
    /** The report's column fit settings (display.columns); absent: the defaults. */
    fits?: Readonly<Record<string, ColumnFit>>
}): ColumnMeasures {
    const { columns, format, levels } = input
    const rows = input.rows.slice(0, MEASURE_ROW_LIMIT)
    const [label, ...others] = columns
    const fits = input.fits ?? NO_FITS
    const measures: ColumnMeasures = {}
    if (label) {
        const heading = headingWidths(label, format)
        const labels = labelWidths(others, rows, format)
        // A label of figures (a typed label, or a figure column first) is never cut.
        const isWhole = labels.isTyped || isFigureColumn(label)
        const chrome = labelChrome(levels)
        const content: Content = {
            natural: Math.max(heading.natural, labels.widest),
            headingMin: heading.min,
            floor: isWhole ? labels.widest : chrome + TEXT_MIN_WIDTH,
            minContent: labels.longestWord,
            pad: chrome,
        }
        const fit = fitOf(fits, label)
        const grow = defaultFlexGrow({ isLabel: true, isFigure: isWhole })
        measures[label.id] = isWhole ? wholeMeasure(content, fit, grow) : fitted(content, fit, grow)
    }
    for (const column of others) {
        const heading = headingWidths(column, format)
        const cells = widestCell(column, rows, format)
        const natural = Math.max(heading.natural, cells.widest)
        const fit = fitOf(fits, column)
        if (isFigureColumn(column)) {
            const whole = {
                natural,
                headingMin: heading.min,
                floor: cells.widest,
                minContent: 0,
                pad: 0,
            }
            measures[column.id] = wholeMeasure(
                whole,
                fit,
                defaultFlexGrow({ isLabel: false, isFigure: true })
            )
            continue
        }
        const content: Content = {
            natural,
            headingMin: heading.min,
            floor: TEXT_MIN_WIDTH + CELL_PADDING,
            minContent: cells.longestWord,
            pad: CELL_PADDING,
        }
        measures[column.id] = fitted(
            content,
            fit,
            defaultFlexGrow({ isLabel: false, isFigure: false })
        )
    }
    return measures
}
