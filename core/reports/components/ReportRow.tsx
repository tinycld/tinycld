import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { ChevronDown, ChevronRight } from 'lucide-react-native'
import type { ReactNode } from 'react'
import { Pressable, Text, View } from 'react-native'
import { isFigureColumn, labelNeed } from '../lib/column-measure'
import { type CellDisplay, cellDisplay, type NumberFormat, rowLabelText } from '../lib/format'
import type { VisibleRow } from '../lib/rows'
import { groupChain } from '../lib/sticky'
import {
    type Block,
    type Justify,
    type LineLayout,
    oneLineLabelSpan,
    oneLineLabelWidth,
    rowLayout,
    type SidePart,
} from '../lib/table-lines'
import type { ResultColumn, ResultRow, RowKind } from '../lib/types'
import {
    CARET_SIZE,
    CONTINUATION_LINE_HEIGHT,
    columnAlign,
    INDENT,
    ROW_HEIGHT,
} from './report-table-layout'

// The rule above a row; it is drawn on the row's container, so it runs across
// the whole row, wrapped lines included.
const ROW_RULE: Record<RowKind, string> = {
    group: '',
    detail: '',
    opening: '',
    subtotal: 'border-t border-border',
    computed: 'border-t border-border',
    total: 'border-t-2 border-foreground',
}

const ROW_WEIGHT: Record<RowKind, string> = {
    group: 'font-semibold',
    detail: '',
    opening: '',
    subtotal: 'font-semibold',
    computed: 'font-semibold',
    total: 'font-semibold',
}

const TEXT_ALIGN: Record<Justify, string> = {
    start: 'text-left',
    center: 'text-center',
    end: 'text-right',
}
export const JUSTIFY_CONTENT = { start: 'flex-start', center: 'center', end: 'flex-end' } as const

/** A one-line amount's alignment: numbers right, anything else left. */
export function amountJustify(column: ResultColumn, index: number): Justify {
    return columnAlign(column, index) === 'right' ? 'end' : 'start'
}

/** Draws a column's cell, aligned as its block says. */
export type RenderCell = (col: string, justify: Justify, minHeight: number) => ReactNode

/**
 * A row (or the heading) as its block tree (lib/table-lines.ts), drawn with flex:
 * a stack is a column of blocks, a side block a row of blocks at the px widths the
 * fit gave them. A side row stretches its blocks to one height, so a cell that
 * spans lines is as tall as the lines beside it; justify aligns each cell, and an
 * empty part keeps the height of the lines it stands for. Heights come from the
 * content: FlashList measures the row.
 */
function BlockView({
    block,
    renderCell,
    lineHeight,
}: {
    block: Block
    renderCell: RenderCell
    lineHeight: number
}) {
    if (block.kind === 'gap') return <View style={{ minHeight: block.lines * lineHeight }} />
    if (block.kind === 'cell')
        return <CellSlot block={block} renderCell={renderCell} lineHeight={lineHeight} />
    if (block.kind === 'stack')
        return <StackBlock parts={block.parts} renderCell={renderCell} lineHeight={lineHeight} />
    return (
        <SideBlock
            of={block.of}
            parts={block.parts}
            renderCell={renderCell}
            lineHeight={lineHeight}
        />
    )
}

function CellSlot({
    block,
    renderCell,
    lineHeight,
}: {
    block: Extract<Block, { kind: 'cell' }>
    renderCell: RenderCell
    lineHeight: number
}) {
    return <View className="min-w-0 grow">{renderCell(block.col, block.justify, lineHeight)}</View>
}

function StackBlock({
    parts,
    renderCell,
    lineHeight,
}: {
    parts: Block[]
    renderCell: RenderCell
    lineHeight: number
}) {
    const items = parts.map((part, i) => (
        <BlockView
            key={blockKey(part, i)}
            block={part}
            renderCell={renderCell}
            lineHeight={lineHeight}
        />
    ))
    return <View className="grow">{items}</View>
}

// A part's px width from the fit; a hand-built tree with none takes its share of the tracks.
function partStyle(part: SidePart, of: number) {
    return part.width === undefined
        ? { width: `${(part.tracks / of) * 100}%` }
        : { width: part.width }
}

function SideBlock({
    of,
    parts,
    renderCell,
    lineHeight,
}: {
    of: number
    parts: SidePart[]
    renderCell: RenderCell
    lineHeight: number
}) {
    const items = parts.map((part, i) => (
        <View key={blockKey(part.block, i)} style={partStyle(part, of)}>
            <BlockView block={part.block} renderCell={renderCell} lineHeight={lineHeight} />
        </View>
    ))
    return <View className="flex-row">{items}</View>
}

const lineStyle = (index: number, indent: number) =>
    index === 0 ? undefined : { paddingLeft: indent }

/**
 * A row's (or the heading's) whole tree: the lines after the first of a stack root
 * start at the indent, under a detail row's label text, as the fit sized them. The
 * first line is firstHeight tall, the lines after it restHeight.
 */
export function RootBlockView({
    root,
    indent,
    renderCell,
    firstHeight,
    restHeight,
}: {
    root: Block
    indent: number
    renderCell: RenderCell
    firstHeight: number
    restHeight: number
}) {
    if (root.kind !== 'stack')
        return <BlockView block={root} renderCell={renderCell} lineHeight={firstHeight} />
    const lines = root.parts.map((part, i) => (
        <View key={blockKey(part, i)} style={lineStyle(i, indent)}>
            <BlockView
                block={part}
                renderCell={renderCell}
                lineHeight={i === 0 ? firstHeight : restHeight}
            />
        </View>
    ))
    return <View className="grow">{lines}</View>
}

/** A block's key: its first cell's column, or its place. */
function blockKey(block: Block, index: number): string {
    if (block.kind === 'cell') return block.col
    if (block.kind === 'stack')
        return block.parts[0] ? blockKey(block.parts[0], index) : `stack-${index}`
    if (block.kind === 'side')
        return block.parts[0] ? blockKey(block.parts[0].block, index) : `side-${index}`
    return `gap-${index}`
}

function GroupCaret({ isOpen }: { isOpen: boolean }) {
    const color = useThemeColor('muted-foreground')
    const Icon = isOpen ? ChevronDown : ChevronRight
    return <Icon size={CARET_SIZE} color={color} />
}

// An opening row's label is a note before the group's own rows, so it is quieter.
function labelTone(kind: RowKind): string {
    return kind === 'opening' ? 'text-muted-foreground' : 'text-foreground'
}

// react-native-web gives a Text of more lines `overflow-wrap: break-word`, which
// splits a word wider than its line. The measure gives a wrapping column its
// longest word; if the estimate is short, the word is clipped, as CSS does.
const WHOLE_WORDS = 'web:break-normal'

function LabelText({ row, text, lines = 1 }: { row: ResultRow; text: string; lines?: number }) {
    return (
        <Text
            numberOfLines={lines}
            className={`min-w-0 flex-1 text-sm ${WHOLE_WORDS} ${labelTone(row.kind)} ${ROW_WEIGHT[row.kind]}`}
        >
            {text}
        </Text>
    )
}

interface RowLabelProps {
    item: VisibleRow
    format: NumberFormat
    onToggleGroup(row: ResultRow): void
    justify?: Justify
    minHeight?: number
    /** Part of the stuck copy: its own test ids, and no tab stop (the row in the list has them). */
    isCopy?: boolean
    /** The lines its text may wrap to (the layout's lines); absent: one. */
    lines?: number
}

// Child rows indent the label only, so the figures stay aligned. It grows to fill
// its slot, as a block cell does.
function labelStyle(row: ResultRow, justify: Justify, minHeight: number) {
    return {
        flexGrow: 1,
        minHeight,
        paddingLeft: INDENT + row.depth * INDENT,
        justifyContent: JUSTIFY_CONTENT[justify],
    }
}

const labelId = (row: ResultRow, isCopy: boolean) =>
    `${isCopy ? 'report-sticky-row' : 'report-row'}-${row.id}`

// A role-button Pressable takes a click, Enter and Space on web and a tap on native.
// The stuck copy's is pointer-only: the row in the list is the accessible toggle.
function GroupToggleLabel({
    item,
    format,
    onToggleGroup,
    justify = 'start',
    minHeight = ROW_HEIGHT,
    isCopy = false,
    lines,
}: RowLabelProps) {
    const { row } = item
    const text = rowLabelText(row, format)
    return (
        <Pressable
            testID={labelId(row, isCopy)}
            accessible={!isCopy}
            focusable={!isCopy}
            tabIndex={isCopy ? -1 : 0}
            accessibilityRole="button"
            accessibilityLabel={text}
            aria-expanded={item.isOpen}
            onPress={() => onToggleGroup(row)}
            className="flex-row items-center gap-1 pr-3"
            style={labelStyle(row, justify, minHeight)}
        >
            <GroupCaret isOpen={item.isOpen} />
            <LabelText row={row} text={text} lines={lines} />
        </Pressable>
    )
}

/** A row's label: a group's is its fold toggle. */
export function RowLabel(props: RowLabelProps) {
    if (props.item.isToggle) return <GroupToggleLabel {...props} />
    const { item, format, justify = 'start', minHeight = ROW_HEIGHT, isCopy = false, lines } = props
    const text = rowLabelText(item.row, format)
    return (
        <View
            testID={labelId(item.row, isCopy)}
            className="flex-row items-center pr-3"
            style={labelStyle(item.row, justify, minHeight)}
        >
            <LabelText row={item.row} text={text} lines={lines} />
        </View>
    )
}

/**
 * A cell's box. A one-line amount keeps its width; a block cell has none and
 * grows to fill its slot, so a cell spanning lines is as tall as they are.
 */
export function cellBoxStyle(width: number | undefined, minHeight: number) {
    return { width, minHeight, flexGrow: width === undefined ? 1 : 0 }
}

function cellTextClass(
    display: CellDisplay,
    justify: Justify,
    kind: RowKind,
    isFigure: boolean
): string {
    const tone = display.isRed || display.isError ? 'text-danger' : 'text-foreground'
    const digits = isFigure ? 'tabular-nums' : ''
    return `text-sm ${WHOLE_WORDS} ${TEXT_ALIGN[justify]} ${digits} ${tone} ${ROW_WEIGHT[kind]}`
}

interface RowCellProps {
    item: VisibleRow
    column: ResultColumn
    format: NumberFormat
    justify: Justify
    onToggleGroup(row: ResultRow): void
    /** A one-line amount's fixed width; a block cell fills its slot. */
    width?: number
    minHeight?: number
    isCopy?: boolean
    /** The lines its text may wrap to (the layout's lines); absent: one. */
    lines?: number
}

// A group row's label is its one accessible toggle. Its figures take the same
// press but are not a button and have no tab stop, so a closed group's figures
// stay readable.
export function RowCell({
    item,
    column,
    format,
    justify,
    onToggleGroup,
    width,
    minHeight = ROW_HEIGHT,
    isCopy = false,
    lines = 1,
}: RowCellProps) {
    const { row } = item
    const display = cellDisplay(column, row, item.cells[column.id], format)
    const text = (
        <Text
            numberOfLines={lines}
            className={cellTextClass(display, justify, row.kind, isFigureColumn(column))}
        >
            {display.text}
        </Text>
    )
    const testID = `${isCopy ? 'report-sticky-cell' : 'report-cell'}-${column.id}-${row.id}`
    const style = cellBoxStyle(width, minHeight)
    if (!item.isToggle) {
        return (
            <View testID={testID} className="justify-center px-3" style={style}>
                {text}
            </View>
        )
    }
    return (
        <Pressable
            testID={testID}
            onPress={() => onToggleGroup(row)}
            accessible={false}
            focusable={false}
            tabIndex={-1}
            className="justify-center px-3"
            style={style}
        >
            {text}
        </Pressable>
    )
}

/**
 * The label column's box on one line: the width the line gave it and the widths of
 * the columns it spans.
 */
export function oneLineLabelStyle(
    layout: LineLayout,
    labelId: string | undefined,
    spanned: readonly string[] = []
) {
    return { width: oneLineLabelWidth(layout, labelId, spanned) }
}

export interface TableRowProps {
    item: VisibleRow
    columns: ResultColumn[]
    layout: LineLayout
    format: NumberFormat
    onToggleGroup(row: ResultRow): void
    isCopy?: boolean
}

// Each column at the width the line gave it; a cell too narrow for its text ends
// in "…" (numberOfLines), or wraps to the lines its fit allows, so nothing runs
// past the table's edge. A group row's
// label also takes the empty cells before its first value (oneLineLabelSpan).
function OneLineRow({ item, columns, layout, format, onToggleGroup, isCopy }: TableRowProps) {
    if (layout.kind !== 'one-line') return null
    const [label, ...amounts] = columns
    const span = oneLineLabelSpan(
        item,
        amounts.map(column => column.id)
    )
    const spanned = amounts.slice(0, span).map(column => column.id)
    const labelBox = oneLineLabelStyle(layout, label?.id, spanned)
    const cells = amounts
        .slice(span)
        .map((column, i) => (
            <RowCell
                key={column.id}
                item={item}
                column={column}
                format={format}
                justify={amountJustify(column, span + i + 1)}
                onToggleGroup={onToggleGroup}
                width={layout.widths[column.id]}
                minHeight={ROW_HEIGHT}
                isCopy={isCopy}
                lines={layout.lines?.[column.id]}
            />
        ))
    return (
        <View className={`flex-row bg-background ${ROW_RULE[item.row.kind]}`}>
            <View style={labelBox}>
                <RowLabel
                    item={item}
                    format={format}
                    onToggleGroup={onToggleGroup}
                    isCopy={isCopy}
                    lines={label ? layout.lines?.[label.id] : undefined}
                />
            </View>
            {cells}
        </View>
    )
}

function BlocksRow({ item, columns, layout, format, onToggleGroup, isCopy }: TableRowProps) {
    const [label] = columns
    const rowLines = rowLayout(item, layout, label?.id ?? '', labelNeed(item, format))
    if (rowLines.kind !== 'blocks') return null
    const byId = new Map(columns.map(column => [column.id, column]))
    const restHeight = item.isToggle ? ROW_HEIGHT : CONTINUATION_LINE_HEIGHT
    const renderCell: RenderCell = (col, justify, minHeight) => {
        if (col === label?.id) {
            return (
                <RowLabel
                    item={item}
                    format={format}
                    onToggleGroup={onToggleGroup}
                    justify={justify}
                    minHeight={minHeight}
                    isCopy={isCopy}
                    lines={rowLines.lines?.[col]}
                />
            )
        }
        const column = byId.get(col)
        if (!column) return null
        return (
            <RowCell
                item={item}
                column={column}
                format={format}
                justify={justify}
                onToggleGroup={onToggleGroup}
                minHeight={minHeight}
                isCopy={isCopy}
                lines={rowLines.lines?.[col]}
            />
        )
    }
    return (
        <View className={`bg-background ${ROW_RULE[item.row.kind]}`}>
            <RootBlockView
                root={rowLines.root}
                indent={rowLines.indent}
                renderCell={renderCell}
                firstHeight={ROW_HEIGHT}
                restHeight={restHeight}
            />
        </View>
    )
}

/** One row of the table: one line, or its block tree where rows wrap (E14). */
export function TableRow(props: TableRowProps) {
    if (props.layout.kind === 'one-line') return <OneLineRow {...props} />
    return <BlocksRow {...props} />
}

interface StickyGroupProps extends Omit<TableRowProps, 'item' | 'isCopy'> {
    rows: VisibleRow[]
    index: number
}

function OwnRow({ item, ...props }: Omit<TableRowProps, 'item'> & { item: VisibleRow | null }) {
    if (!item) return null
    return <TableRow {...props} item={item} />
}

// A stuck label line runs across the row.
const STICKY_LABEL_BOX = { flex: 1 } as const

// The stuck copy of a section boundary (FlashList's StickyHeader target): the
// open groups the rows below it are inside, outermost first, as label lines; an
// open group's own row is drawn whole, figures included, as the list draws it.
// Each label still folds its group on a press, but the copy is hidden from
// assistive technology and the tab order: the rows in the list are the
// accessible controls. Outside every group the copy is empty. FlashList pushes
// the copy up by the height it measures for it, so a stack of groups goes up
// whole when the next section reaches it.
export function StickyGroup({ rows, index, ...props }: StickyGroupProps) {
    const byId = new Map(rows.map(item => [item.row.id, item]))
    const chain = groupChain(rows, index).flatMap(row => byId.get(row.id) ?? [])
    if (chain.length === 0) return null
    const stuck = rows[index]
    const own = chain[chain.length - 1] === stuck ? (stuck ?? null) : null
    const labels = (own ? chain.slice(0, -1) : chain).map(item => (
        <View key={item.row.id} className="flex-row bg-background">
            <View style={STICKY_LABEL_BOX}>
                <RowLabel
                    item={item}
                    format={props.format}
                    onToggleGroup={props.onToggleGroup}
                    isCopy
                />
            </View>
        </View>
    ))
    return (
        <View
            testID="report-sticky-group"
            aria-hidden
            importantForAccessibility="no-hide-descendants"
            accessibilityElementsHidden
            className="border-b border-border bg-background"
        >
            {labels}
            <OwnRow {...props} item={own} isCopy />
        </View>
    )
}
