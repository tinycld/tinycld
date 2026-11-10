import { INDENT } from '../components/report-table-layout'
import {
    CELL_PADDING,
    type ColumnMeasure,
    type ColumnMeasures,
    FLAT_LEVELS,
    measureColumns,
    type RowLevels,
} from './column-measure'
import { DEFAULT_DISPLAY, isRightAligned } from './format'
import type { VisibleRow } from './rows'
import {
    type DisplayCell,
    type DisplayLayout,
    type DisplayOneLine,
    type DisplayWidth,
    parseFitLength,
} from './spec'
import type { CellValue, ResultColumn } from './types'

export { FLAT_LEVELS, isFigureColumn, type RowLevels, rowLevelsOf } from './column-measure'

// E14: a report row wraps onto more lines where its columns do not fit, after
// Gridley: one cell renderer per column, and per width a layout that puts each
// column on a line, at a start track, spanning tracks and lines, with an
// alignment. React Native has no CSS grid on native, so a layout is cut into a
// tree of blocks (guillotine cuts between lines or between tracks) that flex
// draws: a stack of blocks top to bottom, or blocks side by side whose widths
// the fit gives by content. Row content is flex lines; FlashList positions
// rows (R28).

export const MAX_TRACKS = 12
export const MAX_LINES = 8
export const PINWHEEL_MESSAGE =
    "These cells overlap in a way that can't be laid out; move one cell."
export const PIVOT_NOTICE =
    "The pivot columns do not fit this width's arrangement, so the rows are laid out automatically here."
export const ARRANGEMENT_NOTICE =
    "This width's arrangement cannot be laid out, so the rows are laid out automatically here."

export type Justify = 'start' | 'center' | 'end'
export type SidePart = { block: Block; tracks: number; width?: number }
export type Block =
    | { kind: 'cell'; col: string; justify: Justify }
    | { kind: 'gap'; lines: number }
    | { kind: 'stack'; parts: Block[] }
    | { kind: 'side'; of: number; parts: SidePart[] }
/**
 * One line, each column at its width; or blocks whose side parts carry px widths.
 * `lines`: the columns whose text wraps, and to how many lines (absent: one).
 */
export type LineLayout =
    | { kind: 'one-line'; widths: Record<string, number>; lines?: Record<string, number> }
    | {
          kind: 'blocks'
          root: Block
          indent: number
          notice?: string
          lines?: Record<string, number>
      }
/** Which rule laid the rows out: one line, an author arrangement, or automatic lines. */
export type LayoutSource = 'one-line' | DisplayWidth | 'auto'
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
 * found. The client alone places cells; the engine checks each cell's numbers only.
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

// A cell's first refusal before placement (the engine checks the same numbers).
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
 * The placement rules: a cell with a start sits there; one without takes the
 * first tracks of its line free on every line it spans; then the cells must cut
 * down by straight lines. The one copy: the engine leaves placement to the client.
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
function sideParts(block: Block, tracks: number): SidePart[] {
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

// No arrangement fits: the columns in order on lines, first fit at their mins, the
// label first on line 1 (review §7.1-5). Line 1 has the whole width; the later
// lines start at the indent. A column too wide for a line alone gets that line
// and is cut (fitRoot with clamp).
function autoLines(
    columns: readonly ResultColumn[],
    width: number,
    measures: ColumnMeasures,
    indent: number
): Extract<LineLayout, { kind: 'blocks' }> {
    const lines: ResultColumn[][] = []
    const at = measuresAt(measures, width)
    let used = 0
    for (const column of columns) {
        const min = measureOf(at, column.id).min
        const line = lines[lines.length - 1]
        const room = lines.length === 1 ? width : Math.max(0, width - indent)
        if (line && used + min <= room) {
            line.push(column)
            used += min
            continue
        }
        lines.push([column])
        used = min
    }
    const labelId = columns[0]?.id
    const sideOf = (line: ResultColumn[]): Block => ({
        kind: 'side',
        of: line.length,
        parts: line.map(column => ({
            block: {
                kind: 'cell',
                col: column.id,
                justify: column.id === labelId ? 'start' : justifyOf(column),
            },
            tracks: 1,
        })),
    })
    const root = stackOf(lines.map(sideOf))
    return { kind: 'blocks', root: fitRoot(root, width, measures, indent, true) ?? root, indent }
}

// A pivot cell column's entry stands for all its pivot columns, each as wide
// and as tall as the entry, side by side from where the entry is placed and
// wrapping onto the lines below it, from the first track. When the result puts
// the cell column right after its pivot columns (order "spec"), the entry ends
// with the cell column too, so the total reads after them as on one line.
function expandPivots(
    layout: DisplayLayout,
    placed: readonly Placed[],
    columns: readonly ResultColumn[]
): DisplayCell[] {
    return layout.cells.flatMap((cell, index) => {
        const pivots = columns.filter(c => c.pivot?.cell === cell.col)
        const at = placed[index]
        if (pivots.length === 0 || !at) return [cell]
        const ids = pivots.map(p => p.id)
        if (totalFollowsPivots(columns, cell.col)) ids.push(cell.col)
        let track = at.start - 1
        let line = at.line
        return ids.map(col => {
            if (track + at.span > layout.tracks) {
                track = 0
                line += at.lines
            }
            const start = track + 1
            track += at.span
            return { col, line, start, span: at.span, lines: at.lines, justify: cell.justify }
        })
    })
}

function totalFollowsPivots(columns: readonly ResultColumn[], cell: string): boolean {
    const at = columns.findIndex(c => c.id === cell && !c.pivot)
    return at > 0 && columns[at - 1]?.pivot?.cell === cell
}

type Authored = { kind: 'tree'; root: Block } | { kind: 'refused'; notice?: string }

// An arrangement's tree, without widths; or its refusal, so the choice moves on.
function authoredLines(layout: DisplayLayout, columns: readonly ResultColumn[]): Authored {
    const arranged = placeCells(layout.tracks, layout.cells)
    // A spec from the CLI or the API may hold an arrangement the builder would
    // refuse; the engine does not place cells, so the view says why it skips it.
    if ('problem' in arranged) return { kind: 'refused', notice: ARRANGEMENT_NOTICE }
    const cells = expandPivots(layout, arranged.placed, columns)
    // The engine checks the entries as written; a run's pivot columns may still
    // run into another cell or past the last line. The arrangement is then
    // skipped, and when the rows end in automatic lines the table says why.
    const result = placeCells(layout.tracks, cells)
    if ('problem' in result) return { kind: 'refused', notice: PIVOT_NOTICE }
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
        return { kind: 'tree', root: stackOf([label, tree]) }
    }
    return { kind: 'tree', root: tree }
}

const NO_MEASURE: ColumnMeasure = { natural: 0, min: 0, grow: 0 }

export function measureOf(measures: ColumnMeasures, id: string): ColumnMeasure {
    return measures[id] ?? NO_MEASURE
}

/** A column's measure at a line's width: its % settings turned into px. */
export function atWidth(m: ColumnMeasure, lineWidth: number): ColumnMeasure {
    const { pct, ...rest } = m
    if (!pct) return m
    const part = (share: number) => Math.floor(share * lineWidth)
    if (pct.width !== undefined) {
        // A fixed share of the line, never under the column's least width.
        const px = Math.max(rest.min, part(pct.width))
        return { ...rest, natural: px, min: px, max: px }
    }
    const min = pct.min === undefined ? rest.min : Math.max(rest.min, part(pct.min))
    const cap = pct.max === undefined ? rest.max : Math.min(rest.max ?? Infinity, part(pct.max))
    const max = cap === undefined ? undefined : Math.max(min, cap)
    const natural = Math.min(Math.max(rest.natural, min), max ?? Infinity)
    return { ...rest, natural, min, ...(max === undefined ? {} : { max }) }
}

// The measures at a line's width; the same object when no column has a % setting.
function measuresAt(measures: ColumnMeasures, lineWidth: number): ColumnMeasures {
    if (!Object.values(measures).some(m => m.pct)) return measures
    return Object.fromEntries(
        Object.entries(measures).map(([id, m]) => [id, atWidth(m, lineWidth)])
    )
}

const sum = (values: readonly number[]) => values.reduce((total, n) => total + n, 0)

const maxOf = (part: ColumnMeasure) => part.max ?? Number.POSITIVE_INFINITY

// The spare room by grow, in rounds: a part that reaches its cap (its max unless
// another cap is given) keeps it, and the next round shares what is left among the
// others. With no cap it is one round. When every grower is capped, shareWidth
// still gives the rest to the first part (the label), past its max: a row is
// always filled.
function growInto(
    widths: number[],
    parts: readonly ColumnMeasure[],
    width: number,
    capOf: (part: ColumnMeasure) => number = maxOf
) {
    const capped = new Set<number>()
    for (let round = 0; round < parts.length; round++) {
        const spare = width - sum(widths)
        const open = parts.flatMap((part, i) => (part.grow > 0 && !capped.has(i) ? [i] : []))
        const totalGrow = sum(open.map(i => parts[i]?.grow ?? 0))
        if (spare <= 0 || totalGrow === 0) return
        let isCapped = false
        for (const i of open) {
            const part = parts[i]
            if (!part) continue
            const room = Math.max(0, capOf(part) - (widths[i] ?? 0))
            const add = Math.floor((spare * part.grow) / totalGrow)
            widths[i] = (widths[i] ?? 0) + Math.min(add, room)
            if (add >= room) {
                capped.add(i)
                isCapped = true
            }
        }
        if (!isCapped) return
    }
}

const lackOf = (part: ColumnMeasure) => Math.max(0, part.natural - part.min)

// Shares `width` among the parts: each at its min; then what `lack` gives each part,
// in proportion when the room is short; then the rest by grow, first up to each
// part's `firstCap` when one is given, then a part stopping at its max. The parts
// add up to `width` exactly: the rounding remainder goes to the first part (the
// label). Null when the mins do not fit.
function share(
    parts: readonly ColumnMeasure[],
    width: number,
    lack: (part: ColumnMeasure) => number,
    firstCap?: (part: ColumnMeasure) => number
): number[] | null {
    if (parts.length === 0) return []
    const widths = parts.map(part => part.min)
    if (sum(widths) > width) return null
    const lacks = parts.map(lack)
    const totalLack = sum(lacks)
    if (totalLack > 0) {
        const fraction = Math.min(1, (width - sum(widths)) / totalLack)
        for (const [i, need] of lacks.entries())
            widths[i] = (widths[i] ?? 0) + Math.floor(need * fraction)
    }
    if (firstCap) growInto(widths, parts, width, firstCap)
    growInto(widths, parts, width)
    widths[0] = (widths[0] ?? 0) + width - sum(widths)
    return widths
}

// A growing column's natural width, never past its max.
const naturalCap = (part: ColumnMeasure) => Math.min(part.natural, maxOf(part))

/**
 * Widths that share `width` by content, as CSS table-layout:auto does: the spare
 * room first in proportion to what each part lacks of its natural width, then by
 * grow. A side block's parts (arranged and automatic lines), whose grow is their
 * tracks.
 */
export function shareWidth(parts: readonly ColumnMeasure[], width: number): number[] | null {
    return share(parts, width, lackOf)
}

/**
 * The widths of one line (J2, R10): each column at its min, then the spare room by
 * grow (Extra room, `flexGrow`), so a column with twice the grow gets twice the
 * room while both lack room. A growing column stops at its natural width while
 * another growing column is still below its own, and the others share what it
 * leaves by grow, so no text is cut while a short column holds empty room; once
 * every growing column has its natural width, the rest is shared by grow again. A
 * column with no grow gets only what it lacks of its natural width, and no more,
 * so a figure's heading does not wrap when there is room.
 */
export function shareLineWidth(parts: readonly ColumnMeasure[], width: number): number[] | null {
    return share(parts, width, part => (part.grow > 0 ? 0 : lackOf(part)), naturalCap)
}

/**
 * The widths of one line at this width (review §7.1-3), or null when the columns'
 * mins do not fit: figures, dates and short text are never cut (their min is their
 * natural width); long text narrows to its min. The spare room goes by Extra room
 * (shareLineWidth).
 */
export function fitOneLine(
    columns: readonly ResultColumn[],
    width: number,
    measures: ColumnMeasures
): Record<string, number> | null {
    const widths = shareLineWidth(
        columns.map(column => measureOf(measures, column.id)),
        width
    )
    if (!widths) return null
    return Object.fromEntries(columns.map((column, i) => [column.id, widths[i] ?? 0]))
}

type Extent = { min: number; natural: number; max?: number }

// What a block needs: a cell its column's measure, a gap nothing, a stack its
// widest part, a side block its parts side by side.
function extentOf(block: Block, measures: ColumnMeasures): Extent {
    if (block.kind === 'cell') {
        const { min, natural, max } = measureOf(measures, block.col)
        return { min, natural, ...(max === undefined ? {} : { max }) }
    }
    if (block.kind === 'gap') return { min: 0, natural: 0 }
    const extents =
        block.kind === 'stack'
            ? block.parts.map(part => extentOf(part, measures))
            : block.parts.map(part => extentOf(part.block, measures))
    const pick = block.kind === 'stack' ? Math.max : (...n: number[]) => sum(n)
    return {
        min: pick(0, ...extents.map(e => e.min)),
        natural: pick(0, ...extents.map(e => e.natural)),
    }
}

// Widths when the mins do not fit (one automatic line of a single column wider than
// the view): in proportion to the mins, so they still add up to the width.
function clampedWidths(parts: readonly Extent[], width: number): number[] {
    const total = sum(parts.map(part => part.min)) || 1
    const widths = parts.map(part => Math.floor((part.min / total) * width))
    widths[0] = (widths[0] ?? 0) + width - sum(widths)
    return widths
}

/**
 * The block at this width with px widths on every side part (D5): each side block
 * shares its width by content, a part's tracks being its share of the spare room.
 * Null when a part's min does not fit. `clamp` gives a lone cell wider than its
 * width the width it has (automatic lines), so it is cut rather than refused.
 */
function fitBlock(
    block: Block,
    width: number,
    measures: ColumnMeasures,
    clamp = false
): Block | null {
    if (block.kind === 'cell')
        return clamp || measureOf(measures, block.col).min <= width ? block : null
    if (block.kind === 'gap') return block
    if (block.kind === 'stack') {
        const parts: Block[] = []
        for (const part of block.parts) {
            const fitted = fitBlock(part, width, measures, clamp)
            if (!fitted) return null
            parts.push(fitted)
        }
        return { ...block, parts }
    }
    const shares = block.parts.map(part => ({
        ...extentOf(part.block, measures),
        grow: part.tracks,
    }))
    const widths = shareWidth(shares, width) ?? (clamp ? clampedWidths(shares, width) : null)
    if (!widths) return null
    const parts: SidePart[] = []
    for (const [i, part] of block.parts.entries()) {
        const partWidth = widths[i] ?? 0
        const fitted = fitBlock(part.block, partWidth, measures, clamp)
        if (!fitted) return null
        parts.push({ ...part, block: fitted, width: partWidth })
    }
    return { ...block, parts }
}

/** Where the lines after a row's first start: under a detail row's label text. */
export function continuationIndent(levels: RowLevels): number {
    return Math.max(0, INDENT + (levels.groups + levels.nests) * INDENT - CELL_PADDING / 2)
}

// A stack root's first line has the whole width; the lines after it start at the
// indent, so they are fitted to what is left.
function fitRoot(
    root: Block,
    width: number,
    measures: ColumnMeasures,
    indent: number,
    clamp = false
): Block | null {
    if (root.kind !== 'stack') return fitBlock(root, width, measuresAt(measures, width), clamp)
    const parts: Block[] = []
    for (const [i, part] of root.parts.entries()) {
        const lineWidth = i === 0 ? width : Math.max(0, width - indent)
        const fitted = fitBlock(part, lineWidth, measuresAt(measures, lineWidth), clamp)
        if (!fitted) return null
        parts.push(fitted)
    }
    return { ...root, parts }
}

// Arrangements are tried widest first, whatever the screen: the screen's width
// class only names the builder's tabs.
const WIDTH_ORDER: readonly DisplayWidth[] = ['desktop', 'tablet', 'mobile']

export type FitTableInput = {
    columns: readonly ResultColumn[]
    width: number
    layouts: readonly DisplayLayout[]
    /** How deep the report's rows go (rowLevelsOf); absent: a flat report. */
    levels?: RowLevels
    /** The run's measure (measureColumns); absent: the headings and worst cases alone. */
    measures?: ColumnMeasures
    /** The least row width for one line (display.oneLine); absent: any width. */
    oneLine?: DisplayOneLine
}

// A px gate (display.oneLine.minWidth, layouts[].minWidth); 0 when unset.
const gateOf = (minWidth: string | undefined) =>
    minWidth ? (parseFitLength(minWidth)?.value ?? 0) : 0

function linesOf(measures: ColumnMeasures): Record<string, number> | undefined {
    const wrapping = Object.entries(measures).flatMap(([id, m]) =>
        m.lines !== undefined && m.lines > 1 ? [[id, m.lines] as const] : []
    )
    return wrapping.length > 0 ? Object.fromEntries(wrapping) : undefined
}

// The indents an authored arrangement is tried at: the full continuation indent,
// then one group level fewer at a time down to none, then 0. A phone that cannot
// give the figures line its full indent draws the author's lines a little less
// indented rather than falling to automatic lines (which place every column).
function indentsToTry(levels: RowLevels): number[] {
    const fewer = Array.from({ length: levels.groups }, (_, i) =>
        continuationIndent({ groups: levels.groups - 1 - i, nests: 0 })
    )
    return [...new Set([continuationIndent(levels), ...fewer, 0])]
}

/**
 * The lines of every row (and the heading) at this width, by a fit test on the
 * measured width (review §7.1-4): one line when the row is at least the one-line
 * width and its columns' mins fit; else the first author arrangement, widest
 * first, that the width reaches (its minWidth) and whose lines fit, at the full
 * continuation indent or, failing that, at a smaller one (one group level fewer
 * at a time, then 0), each line
 * shared by content; else the automatic lines. Never wider than `width`. Also
 * says which rule it used (the table's e2e marker).
 */
export function fitTable(input: FitTableInput): { layout: LineLayout; source: LayoutSource } {
    const { columns, width, layouts } = input
    const levels = input.levels ?? FLAT_LEVELS
    const measures =
        input.measures ?? measureColumns({ columns, rows: [], format: DEFAULT_DISPLAY, levels })
    const lines = linesOf(measures)
    const withLines = <T extends LineLayout>(layout: T): T =>
        lines ? { ...layout, lines } : layout
    const isOneLineWidth = width >= gateOf(input.oneLine?.minWidth)
    const widths = isOneLineWidth ? fitOneLine(columns, width, measuresAt(measures, width)) : null
    if (widths) return { layout: withLines({ kind: 'one-line', widths }), source: 'one-line' }
    const indent = continuationIndent(levels)
    let notice: string | undefined
    for (const name of WIDTH_ORDER) {
        const layout = layouts.find(l => l.width === name)
        // Below its minWidth an arrangement is not tried; it is not refused, so no notice.
        if (!layout || width < gateOf(layout.minWidth)) continue
        const authored = authoredLines(layout, columns)
        if (authored.kind === 'refused') {
            notice = notice ?? authored.notice
            continue
        }
        for (const at of indentsToTry(levels)) {
            const root = fitRoot(authored.root, width, measures, at)
            if (root)
                return { layout: withLines({ kind: 'blocks', root, indent: at }), source: name }
        }
    }
    const auto = autoLines(columns, width, measures, indent)
    return { layout: withLines(notice ? { ...auto, notice } : auto), source: 'auto' }
}

export function tableLines(input: FitTableInput): LineLayout {
    return fitTable(input).layout
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
function pruned(block: Block, keeps: (col: string) => boolean, labelId: string): Block | null {
    if (block.kind === 'gap') return block
    if (block.kind === 'cell') return keeps(block.col) ? block : null
    if (block.kind === 'stack') {
        const parts = block.parts.flatMap(part => {
            const kept = pruned(part, keeps, labelId)
            return kept && hasCell(kept) ? [kept] : []
        })
        return parts.length > 0 ? stackOf(parts) : null
    }
    const parts = block.parts.map(part => ({
        ...part,
        block: pruned(part.block, keeps, labelId) ?? LINE_GAP,
    }))
    return parts.some(part => hasCell(part.block)) ? labelSpans({ ...block, parts }, labelId) : null
}

const isLabelCell = (block: Block, labelId: string) =>
    block.kind === 'cell' && block.col === labelId

// The label takes the empty parts right of it on its line: their tracks and widths
// become its own (review §7.1-2). A side block left with one part is that block.
export function labelSpans(block: Extract<Block, { kind: 'side' }>, labelId: string): Block {
    const parts: SidePart[] = []
    for (const part of block.parts) {
        const last = parts[parts.length - 1]
        if (last && isLabelCell(last.block, labelId) && part.block.kind === 'gap') {
            parts[parts.length - 1] = {
                ...last,
                tracks: last.tracks + part.tracks,
                width:
                    last.width === undefined || part.width === undefined
                        ? undefined
                        : last.width + part.width,
            }
            continue
        }
        parts.push(part)
    }
    const [only] = parts
    return parts.length === 1 && only ? only.block : { ...block, parts }
}

// H7: a row whose first line is its label alone moves the label into the empty
// parts that start its next line, when they hold the whole label. That line becomes
// the row's first, drawn without the indent, so the label part also takes the indent.
function labelOntoFigures(root: Block, labelId: string, need: number, indent: number): Block {
    if (root.kind !== 'stack') return root
    const [first, next, ...rest] = root.parts
    if (!first || !isLabelCell(first, labelId) || next?.kind !== 'side') return root
    let room = indent
    let tracks = 0
    let count = 0
    for (const part of next.parts) {
        if (part.block.kind !== 'gap' || part.width === undefined) break
        room += part.width
        tracks += part.tracks
        count++
    }
    if (count === 0 || room < need) return root
    const line: Block = {
        ...next,
        parts: [{ block: first, tracks, width: room }, ...next.parts.slice(count)],
    }
    return rest.length === 0 ? line : stackOf([line, ...rest])
}

/**
 * A row's lines. A row with no figures (a group its subtotal follows) draws only
 * its label, across the row. A row that is not a detail row (group, opening,
 * subtotal, total, computed) draws only the columns it has a value in, so a phone
 * opening row is its label line, then its figures line (R14); a detail row keeps
 * every line, so a field sits at the same place in each row.
 */
export function rowLayout(
    item: VisibleRow,
    layout: LineLayout,
    labelId: string,
    need: number
): LineLayout {
    if (layout.kind === 'one-line') return layout
    const labelOnly: Block = { kind: 'cell', col: labelId, justify: 'start' }
    if (Object.keys(item.cells).length === 0)
        return {
            kind: 'blocks',
            root: labelOnly,
            indent: layout.indent,
            ...(layout.lines ? { lines: layout.lines } : {}),
        }
    if (item.row.kind === 'detail') return layout
    const keeps = (col: string) => col === labelId || hasValue(item.cells[col])
    if (blockColumns(layout.root).every(keeps)) return layout
    const root = pruned(layout.root, keeps, labelId) ?? labelOnly
    return { ...layout, root: labelOntoFigures(root, labelId, need, layout.indent) }
}

/**
 * How many columns after the label a row's label spans on one line: a non-detail
 * row (group, opening, subtotal, computed, total) spans its empty cells up to its
 * first value, every column when it has none; a detail row spans none, so its label
 * ends in "…" at the label column's width and every cell keeps its column.
 */
export function oneLineLabelSpan(item: VisibleRow, columnIds: readonly string[]): number {
    if (item.row.kind === 'detail') return 0
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
