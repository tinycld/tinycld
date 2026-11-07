import { FlashList, type ListRenderItemInfo } from '@shopify/flash-list'
import { type Breakpoint, useWindowWidth } from '@tinycld/core/components/workspace/useBreakpoint'
import { useMemo, useState } from 'react'
import { ActivityIndicator, Platform, Text, View } from 'react-native'
import { headingText, type NumberFormat } from '../lib/format'
import type { VisibleRow } from '../lib/rows'
import type { DisplayLayout } from '../lib/spec'
import { stickyIndices } from '../lib/sticky'
import { type Justify, type LineLayout, tableLines } from '../lib/table-lines'
import type { ResultColumn, ResultRow } from '../lib/types'
import {
    amountJustify,
    BlockView,
    cellBoxStyle,
    oneLineLabelStyle,
    type RenderCell,
    StickyGroup,
    TableRow,
    type TableRowProps,
} from './ReportRow'

const TEXT_ALIGN: Record<Justify, string> = {
    start: 'text-left',
    center: 'text-center',
    end: 'text-right',
}

// The value kept from an earlier render while it is equal in content, so a new
// array or object with the same content does not redraw every row.
function useStable<T>(value: T): T {
    const [kept, setKept] = useState(value)
    if (kept === value || JSON.stringify(kept) === JSON.stringify(value)) return kept
    setKept(value)
    return value
}

function HeadingCell({
    column,
    justify,
    width,
    format,
}: {
    column: ResultColumn
    justify: Justify
    width?: number
    format: NumberFormat
}) {
    const text = headingText(column, format)
    return (
        <View
            testID={`report-heading-${column.id}`}
            className="justify-center px-3"
            style={cellBoxStyle(width)}
        >
            <Text
                numberOfLines={1}
                className={`text-xs font-semibold uppercase text-muted-foreground ${TEXT_ALIGN[justify]}`}
            >
                {text}
            </Text>
        </View>
    )
}

// The label's heading sits over the label's text, which starts after the row's indent.
function LabelHeading({ column, format }: { column: ResultColumn; format: NumberFormat }) {
    return (
        <View className="grow pl-1">
            <HeadingCell column={column} justify="start" format={format} />
        </View>
    )
}

interface HeadingRowProps {
    columns: ResultColumn[]
    layout: LineLayout
    format: NumberFormat
}

function OneLineHeading({ columns, layout, format }: HeadingRowProps) {
    if (layout.kind !== 'one-line') return null
    const [label, ...amounts] = columns
    if (!label) return null
    const cells = amounts.map((column, i) => (
        <HeadingCell
            key={column.id}
            column={column}
            justify={amountJustify(column, i + 1)}
            width={layout.widths[column.id]}
            format={format}
        />
    ))
    return (
        <>
            <View style={oneLineLabelStyle(layout, label.id)}>
                <LabelHeading column={label} format={format} />
            </View>
            {cells}
        </>
    )
}

// The heading is drawn by the same blocks as the rows, so each heading sits over
// its values and wraps the same way.
function BlocksHeading({ columns, layout, format }: HeadingRowProps) {
    if (layout.kind !== 'blocks') return null
    const byId = new Map(columns.map(column => [column.id, column]))
    const labelId = columns[0]?.id
    const renderCell: RenderCell = (col, justify) => {
        const column = byId.get(col)
        if (!column) return null
        if (col === labelId) return <LabelHeading column={column} format={format} />
        return <HeadingCell column={column} justify={justify} format={format} />
    }
    return (
        <View className="flex-1">
            <BlockView block={layout.root} renderCell={renderCell} />
        </View>
    )
}

function HeadingCells(props: HeadingRowProps) {
    if (props.layout.kind === 'one-line') return <OneLineHeading {...props} />
    return <BlocksHeading {...props} />
}

// Above the list, outside its vertical scroll, so it always stays at the top. Both
// keep a scroll bar's gutter, so the heading's tracks match the rows'.
function HeadingRow(props: HeadingRowProps) {
    return (
        <View
            testID="report-heading-row"
            className="flex-row border-b border-border bg-background web:[scrollbar-gutter:stable] web:overflow-y-hidden"
        >
            <HeadingCells {...props} />
        </View>
    )
}

function LoadingMore({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return (
        <View testID="report-loading-more" className="items-center py-3">
            <ActivityIndicator />
        </View>
    )
}

function LayoutNotice({ text }: { text: string | undefined }) {
    if (!text) return null
    return (
        <Text testID="report-layout-notice" className="px-3 py-2 text-xs text-muted-foreground">
            {text}
        </Text>
    )
}

// Everything a row draws from: FlashList's extraData, the one signal that redraws
// the rows, so renderItem itself never changes.
interface TableData extends Omit<TableRowProps, 'item'> {
    rows: VisibleRow[]
}

function renderTableItem({ item, index, target, extraData }: ListRenderItemInfo<VisibleRow>) {
    const { rows, ...row }: TableData = extraData
    if (target !== 'StickyHeader') return <TableRow {...row} item={item} />
    return <StickyGroup {...row} rows={rows} index={index} />
}

const rowKey = (item: VisibleRow) => item.row.id
const rowType = (item: VisibleRow) => item.row.kind
const STICKY_CONFIG = { useNativeDriver: Platform.OS !== 'web' }

// A row's width: the list's content, which on web is the list less the scroll
// bar's gutter (scrollbar-gutter: stable, any bar width, none for overlay bars),
// and on native the whole list. The heading row keeps the same gutter.
function RowWidthProbe({ onWidth }: { onWidth(width: number): void }) {
    return <View onLayout={e => onWidth(e.nativeEvent.layout.width)} />
}

interface TableListProps extends Omit<TableRowProps, 'item'> {
    rows: VisibleRow[]
    onNearEnd(): void
    isLoadingMore: boolean
    onRowWidth(width: number): void
}

function TableList({ rows, onNearEnd, isLoadingMore, onRowWidth, ...row }: TableListProps) {
    const { columns, layout, format, onToggleGroup } = row
    const sticky = useMemo(() => stickyIndices(rows), [rows])
    const extraData = useMemo<TableData>(
        () => ({ columns, layout, format, onToggleGroup, rows }),
        [columns, layout, format, onToggleGroup, rows]
    )
    return (
        <>
            <HeadingRow columns={columns} layout={layout} format={format} />
            <FlashList
                data={rows}
                renderItem={renderTableItem}
                keyExtractor={rowKey}
                getItemType={rowType}
                extraData={extraData}
                stickyHeaderIndices={sticky}
                stickyHeaderConfig={STICKY_CONFIG}
                onEndReached={onNearEnd}
                onEndReachedThreshold={0.5}
                className="web:[scrollbar-gutter:stable]"
                ListHeaderComponent={<RowWidthProbe onWidth={onRowWidth} />}
                ListFooterComponent={<LoadingMore isVisible={isLoadingMore} />}
            />
        </>
    )
}

export interface ReportTableProps {
    columns: ResultColumn[]
    rows: VisibleRow[]
    /** The design and params of the run whose rows are shown; a new value starts the list at the top. */
    runKey: string
    format: NumberFormat
    breakpoint: Breakpoint
    layouts: readonly DisplayLayout[]
    onToggleGroup(row: ResultRow): void
    onNearEnd(): void
    isLoadingMore: boolean
}

// FlashList measures every row (wrapped rows have the heights their content
// gives them); onEndReached asks for the next page, through useReportRun's
// guard (hasNextPage and not already fetching). FlashList checks it after each
// commit and again when the rows change, so a first page shorter than the view
// asks for the next one too. The table never scrolls sideways (F10): its line is
// fitted to its width, narrowed, or wrapped (tableLines).
export function ReportTable(props: ReportTableProps) {
    const { rows, runKey, breakpoint, layouts, onToggleGroup, onNearEnd, isLoadingMore } = props
    const columns = useStable(props.columns)
    const format = useStable(props.format)
    const windowWidth = useWindowWidth()
    // The width the rows really get (a builder drawer narrows it), measured: local
    // layout state from onLayout. It is kept here, not in the list, so a new run's
    // list starts at the known width. Until the first measure the table is drawn
    // hidden at the window's width, so a wrong first width is never seen.
    const [rowWidth, setRowWidth] = useState<number>()
    const lineWidth = rowWidth ?? windowWidth
    const layout = useMemo(
        () => tableLines({ columns, width: lineWidth, breakpoint, layouts }),
        [columns, lineWidth, breakpoint, layouts]
    )
    if (columns.length === 0) return null
    const notice = layout.kind === 'blocks' ? layout.notice : undefined
    const visibility = rowWidth === undefined ? 'opacity-0' : ''
    // FlashList keeps the row in view anchored when the rows change, which a live
    // re-run of the same design and params needs; a run with a new design, new
    // params or a new sort is another list, so it starts at the top.
    return (
        <View testID="report-table" className={`min-h-0 flex-1 ${visibility}`}>
            <LayoutNotice text={notice} />
            <TableList
                key={runKey}
                rows={rows}
                columns={columns}
                layout={layout}
                format={format}
                onToggleGroup={onToggleGroup}
                onNearEnd={onNearEnd}
                isLoadingMore={isLoadingMore}
                onRowWidth={setRowWidth}
            />
        </View>
    )
}
