import type { Breakpoint } from '@tinycld/core/components/workspace/useBreakpoint'
import {
    type ColumnMeasure,
    type ColumnMeasures,
    FLAT_LEVELS,
    measureColumns,
    type RowLevels,
} from './column-measure'
import { DEFAULT_DISPLAY, isRightAligned } from './format'
import type { VisibleRow } from './rows'
import type { DisplayCell, DisplayLayout, DisplayWidth } from './spec'
import type { CellValue, ResultColumn } from './types'

export { FLAT_LEVELS, isFigureColumn, type RowLevels, rowLevelsOf } from './column-measure'

// E14: a report row wraps onto more lines where its columns do not fit, after
// Gridley: one cell renderer per column, and per width a layout that puts each
// column on a line, at a start track, spanning tracks and lines, with an
// alignment. React Native has no CSS grid on native, so a layout is cut into a
// tree of blocks (guillotine cuts between lines or between tracks) that flex
// draws: a stack of blocks top to bottom, or blocks side by side whose widths
// are their shares of the tracks. Row content is flex lines; FlashList
// positions rows (R28).

export const AUTO_TRACK_MIN = 104
export const MAX_TRACKS = 12
export const MAX_LINES = 8
export const PINWHEEL_MESSAGE =
    "These cells overlap in a way that can't be laid out; move one cell."
export const PIVOT_NOTICE =
    "The pivot columns do not fit this width's arrangement, so the rows are laid out automatically here."

export type Justify = 'start' | 'center' | 'end'
export type Block =
    | { kind: 'cell'; col: string; justify: Justify }
    | { kind: 'gap'; lines: number }
    | { kind: 'stack'; parts: Block[] }
    | { kind: 'side'; of: number; parts: { block: Block; tracks: number }[] }
/** One line, each column at its width (the label's is its minimum: it takes what is left). */
export type LineLayout =
    | { kind: 'one-line'; widths: Record<string, number> }
    | { kind: 'blocks'; root: Block; notice?: string }
export type Placed = { start: number; span: number; line: number; lines: number }
export type Placement = { placed: Placed[] } | { problem: { index: number; message: string } }

// Tracks [x0, x1) and lines [y0, y1), from 0.
type Box = { x0: number; x1: number; y0: number; y1: number }
type Piece = Box & { index: number }

const pieceOf = (p: Placed, index: number): Piece => ({
    index,
    x0: p.start - 1,
    x1: p.start - 1 + p.span,
    y0: p.line - 1,
    y1: p.line - 1 + p.lines,
})

// The two sides of a straight cut at `at` (across: between lines, else between
// tracks), or null when a piece straddles it. A side may be empty.
function split(pieces: readonly Piece[], at: number, across: boolean): [Piece[], Piece[]] | null {
    const lo: Piece[] = []
    const hi: Piece[] = []
    for (const p of pieces) {
        const [a0, a1] = across ? [p.y0, p.y1] : [p.x0, p.x1]
        if (a1 <= at) lo.push(p)
        else if (a0 >= at) hi.push(p)
        else return null
    }
    return [lo, hi]
}

// A cut that parts the pieces: neither side is empty.
function parts(pieces: readonly Piece[], at: number, across: boolean) {
    const sides = split(pieces, at, across)
    return sides && sides[0].length > 0 && sides[1].length > 0 ? sides : null
}

/** A cell's tracks and lines; an absent or zero span or lines counts as one. */
export function extent(c: Pick<DisplayCell, 'span' | 'lines'>) {
    return { span: Math.max(c.span ?? 0, 1), lines: Math.max(c.lines ?? 0, 1) }
}

/**
 * The index of the first cell of a part no straight cut can split, or -1 when
 * the pieces cut down to one each. Lines first, then tracks, the first cut
 * found, as the engine's cutDown (core/server/reports/display.go).
 */
function cutDown(pieces: readonly Piece[], box: Box): number {
    if (pieces.length <= 1) return -1
    for (let y = box.y0 + 1; y < box.y1; y++) {
        const sides = parts(pieces, y, true)
        if (!sides) continue
        const stuck = cutDown(sides[0], { ...box, y1: y })
        return stuck >= 0 ? stuck : cutDown(sides[1], { ...box, y0: y })
    }
    for (let x = box.x0 + 1; x < box.x1; x++) {
        const sides = parts(pieces, x, false)
        if (!sides) continue
        const stuck = cutDown(sides[0], { ...box, x1: x })
        return stuck >= 0 ? stuck : cutDown(sides[1], { ...box, x0: x })
    }
    return Math.min(...pieces.map(p => p.index))
}

// A cell's first refusal before placement, as the engine's checks in order.
function cellProblem(c: DisplayCell, tracks: number): string | null {
    const { span, lines } = extent(c)
    const start = c.start ?? 0
    if (c.line < 1 || c.line > MAX_LINES) return `line is 1 to ${MAX_LINES}`
    if ((c.lines ?? 0) < 0 || lines > MAX_LINES) return `lines is 1 to ${MAX_LINES}`
    if (c.line + lines - 1 > MAX_LINES) return `a cell ends by line ${MAX_LINES}`
    if ((c.span ?? 0) < 0 || span > tracks) return `span is 1 to ${tracks}`
    if (start < 0 || start > tracks || (start > 0 && start + span - 1 > tracks))
        return 'the cell does not fit its line'
    return null
}

/**
 * The engine's placement rules (core/server/reports/display.go placeCells): a
 * cell with a start sits there; one without takes the first tracks of its line
 * free on every line it spans; then the cells must cut down by straight lines.
 */
export function placeCells(tracks: number, cells: readonly DisplayCell[]): Placement {
    const taken = new Set<string>()
    const placed: Placed[] = []
    for (const [index, c] of cells.entries()) {
        const problem = cellProblem(c, tracks)
        if (problem) return { problem: { index, message: problem } }
        const { span, lines } = extent(c)
        const free = (start: number) => {
            for (let line = c.line; line < c.line + lines; line++)
                for (let t = start; t < start + span; t++)
                    if (taken.has(`${line}:${t}`)) return false
            return true
        }
        let start = c.start ?? 0
        if (start === 0) {
            for (let s = 1; s + span - 1 <= tracks && start === 0; s++) if (free(s)) start = s
            if (start === 0) return { problem: { index, message: `no room on line ${c.line}` } }
        } else if (!free(start)) {
            return { problem: { index, message: 'the cell overlaps another' } }
        }
        for (let line = c.line; line < c.line + lines; line++)
            for (let t = start; t < start + span; t++) taken.add(`${line}:${t}`)
        placed.push({ start, span, line: c.line, lines })
    }
    const pieces = placed.map(pieceOf)
    const y1 = Math.max(0, ...pieces.map(p => p.y1))
    const stuck = cutDown(pieces, { x0: 0, x1: tracks, y0: 0, y1 })
    if (stuck >= 0) return { problem: { index: stuck, message: PINWHEEL_MESSAGE } }
    return { placed }
}

/** A stack with nested stacks folded in; a stack of one block is that block. */
function stackOf(parts: Block[]): Block {
    const flat = parts.flatMap(p => (p.kind === 'stack' ? p.parts : [p]))
    const [only] = flat
    return flat.length === 1 && only ? only : { kind: 'stack', parts: flat }
}

/** A part of a side block; a nested side block's parts fold into it (track counts are absolute). */
function sideParts(block: Block, tracks: number): { block: Block; tracks: number }[] {
    return block.kind === 'side' ? block.parts : [{ block, tracks }]
}

const fills = (p: Piece, box: Box) =>
    p.x0 === box.x0 && p.x1 === box.x1 && p.y0 === box.y0 && p.y1 === box.y1

// The block tree of pieces inside a box: a piece that fills its box is a cell,
// an empty box a gap as tall as its lines, anything else is cut where the
// engine cuts it (lines first, then tracks, the first cut found). Always
// cuttable after placeCells: a lone piece smaller than its box is cut at its own
// edges (one side empty), and pieces with no cut are refused as a pinwheel.
function build(pieces: readonly Piece[], box: Box, cellOf: (index: number) => Block): Block {
    const [only] = pieces
    const gap: Block = { kind: 'gap', lines: box.y1 - box.y0 }
    if (!only) return gap
    if (pieces.length === 1 && fills(only, box)) return cellOf(only.index)
    for (let y = box.y0 + 1; y < box.y1; y++) {
        const sides = split(pieces, y, true)
        if (!sides) continue
        const [lo, hi] = sides
        return stackOf([build(lo, { ...box, y1: y }, cellOf), build(hi, { ...box, y0: y }, cellOf)])
    }
    for (let x = box.x0 + 1; x < box.x1; x++) {
        const sides = split(pieces, x, false)
        if (!sides) continue
        const [lo, hi] = sides
        const left = build(lo, { ...box, x1: x }, cellOf)
        const right = build(hi, { ...box, x0: x }, cellOf)
        return {
            kind: 'side',
            of: box.x1 - box.x0,
            parts: [...sideParts(left, x - box.x0), ...sideParts(right, box.x1 - x)],
        }
    }
    return gap
}

const justifyOf = (column: ResultColumn): Justify => (isRightAligned(column) ? 'end' : 'start')

/**
 * How a placed column aligns when its cell does not say: the label column at the
 * left, numbers at the right, the rest at the left. A pivot cell column aligns
 * as its pivot columns; a column not in the result yet at the left.
 */
export function defaultJustify(columns: readonly ResultColumn[], col: string): Justify {
    if (columns[0]?.id === col) return 'start'
    const column = columns.find(c => c.id === col) ?? columns.find(c => c.pivot?.cell === col)
    return column ? justifyOf(column) : 'start'
}

// No arrangement for this width: the label across line 1, the other columns
// in order on the lines below, one per track of at least AUTO_TRACK_MIN.
function autoLines(
    columns: readonly ResultColumn[],
    width: number
): Extract<LineLayout, { kind: 'blocks' }> {
    const [label, ...amounts] = columns
    const tracks = Math.max(1, Math.floor(width / AUTO_TRACK_MIN))
    const lines: Block[] = label ? [{ kind: 'cell', col: label.id, justify: 'start' }] : []
    for (let i = 0; i < amounts.length; i += tracks) {
        const line = amounts.slice(i, i + tracks)
        const parts: { block: Block; tracks: number }[] = line.map(column => ({
            block: { kind: 'cell', col: column.id, justify: justifyOf(column) },
            tracks: 1,
        }))
        // A short last line keeps each amount on its own track.
        if (line.length < tracks)
            parts.push({ block: { kind: 'gap', lines: 1 }, tracks: tracks - line.length })
        lines.push({ kind: 'side', of: tracks, parts })
    }
    return { kind: 'blocks', root: stackOf(lines) }
}

// A pivot cell column's entry stands for all its pivot columns, each as wide
// and as tall as the entry, side by side from where the entry is placed and
// wrapping onto the lines below it, from the first track.
function expandPivots(
    layout: DisplayLayout,
    placed: readonly Placed[],
    columns: readonly ResultColumn[]
): DisplayCell[] {
    return layout.cells.flatMap((cell, index) => {
        const pivots = columns.filter(c => c.pivot?.cell === cell.col)
        const at = placed[index]
        if (pivots.length === 0 || !at) return [cell]
        let track = at.start - 1
        let line = at.line
        return pivots.map(p => {
            if (track + at.span > layout.tracks) {
                track = 0
                line += at.lines
            }
            const start = track + 1
            track += at.span
            return { col: p.id, line, start, span: at.span, lines: at.lines, justify: cell.justify }
        })
    })
}

function authoredLines(
    layout: DisplayLayout,
    columns: readonly ResultColumn[],
    width: number
): LineLayout {
    const arranged = placeCells(layout.tracks, layout.cells)
    if ('problem' in arranged) return autoLines(columns, width)
    const cells = expandPivots(layout, arranged.placed, columns)
    // The engine checks the entries as written; a run's pivot columns may still
    // run into another cell or past the last line. The rows are then laid out
    // automatically, and the table says why.
    const result = placeCells(layout.tracks, cells)
    if ('problem' in result) return { ...autoLines(columns, width), notice: PIVOT_NOTICE }
    const pieces = result.placed.map(pieceOf)
    const lines = Math.max(1, ...pieces.map(p => p.y1))
    const labelId = columns[0]?.id
    const cellOf = (index: number): Block => {
        const cell = cells[index]
        // A placed column the run does not show (hidden by the reader, or a pivot
        // with no columns) keeps its place and its lines, so the others do not move.
        if (!cell || !columns.some(c => c.id === cell.col))
            return { kind: 'gap', lines: result.placed[index]?.lines ?? 1 }
        // The label column starts at the left, whatever its type, as in one line.
        const justify = cell.justify ?? defaultJustify(columns, cell.col)
        return { kind: 'cell', col: cell.col, justify }
    }
    const tree = build(pieces, { x0: 0, x1: layout.tracks, y0: 0, y1: lines }, cellOf)
    // An arrangement that leaves out the label column puts the label on a line of
    // its own above it: a row is never without its label.
    if (labelId && !cells.some(c => c.col === labelId)) {
        const label: Block = { kind: 'cell', col: labelId, justify: 'start' }
        return { kind: 'blocks', root: stackOf([label, tree]) }
    }
    return { kind: 'blocks', root: tree }
}

const NO_MEASURE: ColumnMeasure = { natural: 0, min: 0, grow: 0 }

export function measureOf(measures: ColumnMeasures, id: string): ColumnMeasure {
    return measures[id] ?? NO_MEASURE
}

const sum = (values: readonly number[]) => values.reduce((total, n) => total + n, 0)

/**
 * Widths that share `width` by content, as CSS table-layout:auto does: each part at
 * its min; the spare room first in proportion to what each part lacks of its
 * natural width, then by grow. The parts add up to `width` exactly: the rounding
 * remainder goes to the first part (the label). Null when the mins do not fit.
 */
export function shareWidth(parts: readonly ColumnMeasure[], width: number): number[] | null {
    if (parts.length === 0) return []
    const widths = parts.map(part => part.min)
    if (sum(widths) > width) return null
    const lack = parts.map(part => Math.max(0, part.natural - part.min))
    const totalLack = sum(lack)
    if (totalLack > 0) {
        const share = Math.min(1, (width - sum(widths)) / totalLack)
        for (const [i, need] of lack.entries())
            widths[i] = (widths[i] ?? 0) + Math.floor(need * share)
    }
    const spare = width - sum(widths)
    const totalGrow = sum(parts.map(part => part.grow))
    if (spare > 0 && totalGrow > 0) {
        for (const [i, part] of parts.entries())
            widths[i] = (widths[i] ?? 0) + Math.floor((spare * part.grow) / totalGrow)
    }
    widths[0] = (widths[0] ?? 0) + width - sum(widths)
    return widths
}

/**
 * The widths of one line at this width (review §7.1-3), or null when the columns'
 * mins do not fit: figures, dates and short text are never cut (their min is their
 * natural width); long text narrows to its min, in proportion to what it lacks.
 */
export function fitOneLine(
    columns: readonly ResultColumn[],
    width: number,
    measures: ColumnMeasures
): Record<string, number> | null {
    const widths = shareWidth(
        columns.map(column => measureOf(measures, column.id)),
        width
    )
    if (!widths) return null
    return Object.fromEntries(columns.map((column, i) => [column.id, widths[i] ?? 0]))
}

// The authored widths tried when this width has none and its line does not fit,
// nearest first (R13).
const NARROWER: Record<Breakpoint, readonly DisplayWidth[]> = {
    desktop: ['tablet', 'mobile'],
    tablet: ['mobile'],
    mobile: [],
}

/**
 * The lines of every row (and the heading) at this width (F10): the author's
 * arrangement for this width (Q1); else one line, narrowed if it must be; else the
 * nearest narrower arrangement; else the automatic lines. Never wider than `width`,
 * so the table never scrolls sideways.
 */
export function tableLines(input: {
    columns: readonly ResultColumn[]
    width: number
    breakpoint: Breakpoint
    layouts: readonly DisplayLayout[]
    /** How deep the report's rows go (rowLevelsOf); absent: a flat report. */
    levels?: RowLevels
    /** What each column's content needs (measureColumns); absent: the headings and worst cases alone. */
    measures?: ColumnMeasures
}): LineLayout {
    const { columns, width, breakpoint, layouts, levels } = input
    const own = layouts.find(l => l.width === breakpoint)
    if (own) return authoredLines(own, columns, width)
    const measured =
        input.measures ??
        measureColumns({
            columns,
            rows: [],
            format: DEFAULT_DISPLAY,
            levels: levels ?? FLAT_LEVELS,
        })
    const widths = fitOneLine(columns, width, measured)
    if (widths) return { kind: 'one-line', widths }
    const narrower = NARROWER[breakpoint]
        .map(w => layouts.find(l => l.width === w))
        .find(layout => layout !== undefined)
    if (narrower) return authoredLines(narrower, columns, width)
    return autoLines(columns, width)
}

const hasValue = (value: CellValue | undefined) => value !== undefined && value !== null
const LINE_GAP: Block = { kind: 'gap', lines: 1 }

function hasCell(block: Block): boolean {
    if (block.kind === 'cell') return true
    if (block.kind === 'gap') return false
    if (block.kind === 'stack') return block.parts.some(hasCell)
    return block.parts.some(part => hasCell(part.block))
}

function blockColumns(block: Block): string[] {
    if (block.kind === 'cell') return [block.col]
    if (block.kind === 'gap') return []
    if (block.kind === 'stack') return block.parts.flatMap(blockColumns)
    return block.parts.flatMap(part => blockColumns(part.block))
}

// The block without the cells `keeps` refuses: in a side block a refused cell
// leaves a gap, so the others keep their tracks; a stack line, or a side block,
// left with no cell goes (R14). Null when nothing is left.
function pruned(block: Block, keeps: (col: string) => boolean): Block | null {
    if (block.kind === 'gap') return block
    if (block.kind === 'cell') return keeps(block.col) ? block : null
    if (block.kind === 'stack') {
        const parts = block.parts.flatMap(part => {
            const kept = pruned(part, keeps)
            return kept && hasCell(kept) ? [kept] : []
        })
        return parts.length > 0 ? stackOf(parts) : null
    }
    const parts = block.parts.map(part => ({
        block: pruned(part.block, keeps) ?? LINE_GAP,
        tracks: part.tracks,
    }))
    return parts.some(part => hasCell(part.block)) ? { ...block, parts } : null
}

/**
 * A row's lines. A row with no figures (a group its subtotal follows) draws only
 * its label, across the row. A row that is not a detail row (group, opening,
 * subtotal, total, computed) draws only the columns it has a value in, so a phone
 * opening row is its label line, then its figures line (R14); a detail row keeps
 * every line, so a field sits at the same place in each row.
 */
export function rowLayout(item: VisibleRow, layout: LineLayout, labelId: string): LineLayout {
    if (layout.kind === 'one-line') return layout
    const labelOnly: Block = { kind: 'cell', col: labelId, justify: 'start' }
    if (Object.keys(item.cells).length === 0) return { kind: 'blocks', root: labelOnly }
    if (item.row.kind === 'detail') return layout
    const keeps = (col: string) => col === labelId || hasValue(item.cells[col])
    if (blockColumns(layout.root).every(keeps)) return layout
    return { ...layout, root: pruned(layout.root, keeps) ?? labelOnly }
}

/**
 * How many columns after the label a row's label spans on one line (F10): a group
 * row spans its empty cells up to its first value (every column when it has none),
 * as its blocks lines draw only its label; any other row spans none, so a detail
 * label ends in "…" at the label column's width and every cell keeps its column.
 */
export function oneLineLabelSpan(item: VisibleRow, columnIds: readonly string[]): number {
    if (item.row.kind !== 'group') return 0
    const first = columnIds.findIndex(id => hasValue(item.cells[id]))
    return first === -1 ? columnIds.length : first
}

/**
 * The least width of a one-line row's label box: the label column's width and the
 * widths of the columns it spans (oneLineLabelSpan), so the cells after it keep
 * their places. Undefined where rows are not one line.
 */
export function oneLineLabelWidth(
    layout: LineLayout,
    labelId: string | undefined,
    spanned: readonly string[]
): number | undefined {
    if (layout.kind !== 'one-line' || !labelId) return undefined
    const own = layout.widths[labelId]
    if (own === undefined) return undefined
    return spanned.reduce((sum, id) => sum + (layout.widths[id] ?? 0), own)
}

/** A first arrangement: the first column across line 1, the others in order on the lines below. */
export function startingLayout(
    width: DisplayWidth,
    columns: readonly { id: string }[],
    tracks = 3
): DisplayLayout {
    const [first, ...rest] = columns
    const cells: DisplayCell[] = first ? [{ col: first.id, line: 1, span: tracks }] : []
    for (const [k, c] of rest.entries()) cells.push({ col: c.id, line: 2 + Math.floor(k / tracks) })
    return { width, tracks, cells }
}
