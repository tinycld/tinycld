import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { ChevronDown, ChevronRight } from 'lucide-react-native'
import type { ReactNode } from 'react'
import { Pressable, Text, View } from 'react-native'
import { type CellDisplay, cellDisplay, type NumberFormat, rowLabelText } from '../lib/format'
import type { VisibleRow } from '../lib/rows'
import { groupChain } from '../lib/sticky'
import { type Block, type Justify, type LineLayout, rowLayout } from '../lib/table-lines'
import type { ResultColumn, ResultRow, RowKind } from '../lib/types'
import { columnAlign, INDENT, ROW_HEIGHT } from './report-table-layout'

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
    end: 'text-right tabular-nums',
}
const JUSTIFY_CONTENT = { start: 'flex-start', center: 'center', end: 'flex-end' } as const

const CARET_SIZE = 14

/** A one-line amount's alignment: numbers right, anything else left. */
export function amountJustify(column: ResultColumn, index: number): Justify {
    return columnAlign(column, index) === 'right' ? 'end' : 'start'
}

/** Draws a column's cell, aligned as its block says. */
export type RenderCell = (col: string, justify: Justify) => ReactNode

/**
 * A row (or the heading) as its block tree (lib/table-lines.ts), drawn with flex:
 * a stack is a column of blocks, a side block a row of blocks as wide as their
 * share of the tracks. A side row stretches its blocks to one height, so a cell
 * that spans lines is as tall as the lines beside it; justify aligns each cell,
 * and an empty part keeps the height of the lines it stands for. Heights come
 * from the content: FlashList measures the row.
 */
export function BlockView({ block, renderCell }: { block: Block; renderCell: RenderCell }) {
    if (block.kind === 'gap') return <View style={{ minHeight: block.lines * ROW_HEIGHT }} />
    if (block.kind === 'cell') return <CellSlot block={block} renderCell={renderCell} />
    if (block.kind === 'stack') return <StackBlock parts={block.parts} renderCell={renderCell} />
    return <SideBlock of={block.of} parts={block.parts} renderCell={renderCell} />
}

function CellSlot({
    block,
    renderCell,
}: {
    block: Extract<Block, { kind: 'cell' }>
    renderCell: RenderCell
}) {
    return <View className="min-w-0 grow">{renderCell(block.col, block.justify)}</View>
}

function StackBlock({ parts, renderCell }: { parts: Block[]; renderCell: RenderCell }) {
    const items = parts.map((part, i) => (
        <BlockView key={blockKey(part, i)} block={part} renderCell={renderCell} />
    ))
    return <View className="grow">{items}</View>
}

function SideBlock({
    of,
    parts,
    renderCell,
}: {
    of: number
    parts: { block: Block; tracks: number }[]
    renderCell: RenderCell
}) {
    const items = parts.map((part, i) => (
        <View key={blockKey(part.block, i)} style={{ width: `${(part.tracks / of) * 100}%` }}>
            <BlockView block={part.block} renderCell={renderCell} />
        </View>
    ))
    return <View className="flex-row">{items}</View>
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

function LabelText({ row, text }: { row: ResultRow; text: string }) {
    return (
        <Text
            numberOfLines={1}
            className={`min-w-0 flex-1 text-sm ${labelTone(row.kind)} ${ROW_WEIGHT[row.kind]}`}
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
    /** Part of the stuck copy: its own test ids, and no tab stop (the row in the list has them). */
    isCopy?: boolean
}

// Child rows indent the label only, so the figures stay aligned. It grows to fill its slot, as a block cell does.
function labelStyle(row: ResultRow, justify: Justify) {
    return {
        flexGrow: 1,
        minHeight: ROW_HEIGHT,
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
    isCopy = false,
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
            style={labelStyle(row, justify)}
        >
            <GroupCaret isOpen={item.isOpen} />
            <LabelText row={row} text={text} />
        </Pressable>
    )
}

/** A row's label: a group's is its fold toggle. */
export function RowLabel(props: RowLabelProps) {
    if (props.item.isToggle) return <GroupToggleLabel {...props} />
    const { item, format, justify = 'start', isCopy = false } = props
    const text = rowLabelText(item.row, format)
    return (
        <View
            testID={labelId(item.row, isCopy)}
            className="flex-row items-center pr-3"
            style={labelStyle(item.row, justify)}
        >
            <LabelText row={item.row} text={text} />
        </View>
    )
}

/**
 * A cell's box. A one-line amount keeps its width; a block cell has none and
 * grows to fill its slot, so a cell spanning lines is as tall as they are.
 */
export function cellBoxStyle(width?: number) {
    return { width, minHeight: ROW_HEIGHT, flexGrow: width === undefined ? 1 : 0 }
}

function cellTextClass(display: CellDisplay, justify: Justify, kind: RowKind): string {
    const tone = display.isRed || display.isError ? 'text-danger' : 'text-foreground'
    return `text-sm ${TEXT_ALIGN[justify]} ${tone} ${ROW_WEIGHT[kind]}`
}

interface RowCellProps {
    item: VisibleRow
    column: ResultColumn
    format: NumberFormat
    justify: Justify
    onToggleGroup(row: ResultRow): void
    /** A one-line amount's fixed width; a block cell fills its slot. */
    width?: number
    isCopy?: boolean
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
    isCopy = false,
}: RowCellProps) {
    const { row } = item
    const display = cellDisplay(column, row, item.cells[column.id], format)
    const text = (
        <Text numberOfLines={1} className={cellTextClass(display, justify, row.kind)}>
            {display.text}
        </Text>
    )
    const testID = `${isCopy ? 'report-sticky-cell' : 'report-cell'}-${column.id}-${row.id}`
    const style = cellBoxStyle(width)
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
 * The label column's box on one line (F10): it takes the width the other columns
 * leave, and never less than the width the line gave it.
 */
export function oneLineLabelStyle(layout: LineLayout, labelId: string | undefined) {
    const minWidth = layout.kind === 'one-line' && labelId ? layout.widths[labelId] : undefined
    return { flex: 1, minWidth }
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
// in "…" (numberOfLines 1), so nothing runs past the table's edge.
function OneLineRow({ item, columns, layout, format, onToggleGroup, isCopy }: TableRowProps) {
    if (layout.kind !== 'one-line') return null
    const [label, ...amounts] = columns
    const cells = amounts.map((column, i) => (
        <RowCell
            key={column.id}
            item={item}
            column={column}
            format={format}
            justify={amountJustify(column, i + 1)}
            onToggleGroup={onToggleGroup}
            width={layout.widths[column.id]}
            isCopy={isCopy}
        />
    ))
    return (
        <View className={`flex-row bg-background ${ROW_RULE[item.row.kind]}`}>
            <View style={oneLineLabelStyle(layout, label?.id)}>
                <RowLabel
                    item={item}
                    format={format}
                    onToggleGroup={onToggleGroup}
                    isCopy={isCopy}
                />
            </View>
            {cells}
        </View>
    )
}

function BlocksRow({ item, columns, layout, format, onToggleGroup, isCopy }: TableRowProps) {
    const [label] = columns
    const lines = rowLayout(item, layout, label?.id ?? '')
    if (lines.kind !== 'blocks') return null
    const byId = new Map(columns.map(column => [column.id, column]))
    const renderCell: RenderCell = (col, justify) => {
        if (col === label?.id) {
            return (
                <RowLabel
                    item={item}
                    format={format}
                    onToggleGroup={onToggleGroup}
                    justify={justify}
                    isCopy={isCopy}
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
                isCopy={isCopy}
            />
        )
    }
    return (
        <View className={`bg-background ${ROW_RULE[item.row.kind]}`}>
            <BlockView block={lines.root} renderCell={renderCell} />
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
    const labelBox = oneLineLabelStyle(props.layout, props.columns[0]?.id)
    const labels = (own ? chain.slice(0, -1) : chain).map(item => (
        <View key={item.row.id} className="flex-row bg-background">
            <View style={labelBox}>
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
