import { FlashList, type ListRenderItemInfo } from '@shopify/flash-list'
import { type Breakpoint, useWindowWidth } from '@tinycld/core/components/workspace/useBreakpoint'
import { type ReactNode, useMemo, useRef, useState } from 'react'
import {
    ActivityIndicator,
    Animated,
    Platform,
    ScrollView,
    type ScrollViewInstance,
    Text,
    View,
} from 'react-native'
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
    FrozenLabel,
    type RenderCell,
    StickyGroup,
    TableRow,
    type TableRowProps,
} from './ReportRow'
import { fitsOnOneLine, type ReportTableLayout, reportTableLayout } from './report-table-layout'

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
    oneLine: ReportTableLayout
    format: NumberFormat
    scrollX: Animated.Value | null
}

function OneLineHeading({ columns, oneLine, format, scrollX }: HeadingRowProps) {
    const [label, ...amounts] = columns
    if (!label) return null
    const cells = amounts.map((column, i) => (
        <HeadingCell
            key={column.id}
            column={column}
            justify={amountJustify(column, i + 1)}
            width={oneLine.amountWidth}
            format={format}
        />
    ))
    return (
        <>
            <FrozenLabel scrollX={scrollX} style={oneLine.labelStyle}>
                <LabelHeading column={label} format={format} />
            </FrozenLabel>
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

// Above the list, outside its vertical scroll, so it always stays at the top; on
// a desktop it shares the sideways scroller with the list, so it moves in step.
// Both keep a scroll bar's gutter, so the heading's tracks match the rows'.
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

interface SidewaysProps {
    isEnabled: boolean
    minWidth: number
    /** Draws the table; the offset is null when the table does not scroll sideways. */
    children(scrollX: Animated.Value | null): ReactNode
}

// A desktop table wider than its view scrolls sideways as a whole: the heading
// row and the list sit in one horizontal scroller, and the list scrolls
// vertically inside it. The scroller owns its sideways offset, which drives the
// frozen label column. It stays mounted, and scrollable, when sideways scroll
// turns off (a narrower width), so the list keeps its vertical position and the
// page keeps the browser's touch gestures (a disabled scroller blocks them on
// web); its content is then the view's width, so nothing moves sideways, and its
// offset goes back to 0.
function Sideways({ isEnabled, minWidth, children }: SidewaysProps) {
    const scrollX = useRef(new Animated.Value(0)).current
    const scrollerRef = useRef<ScrollViewInstance>(null)
    const onScroll = Animated.event([{ nativeEvent: { contentOffset: { x: scrollX } } }], {
        useNativeDriver: false,
    })
    const onContentSizeChange = () => {
        if (isEnabled) return
        scrollerRef.current?.scrollTo({ x: 0, animated: false })
        scrollX.setValue(0)
    }
    const content = isEnabled
        ? { width: '100%', minWidth, flexDirection: 'column' as const }
        : { width: '100%', flexDirection: 'column' as const }
    return (
        <ScrollView
            ref={scrollerRef}
            horizontal
            showsHorizontalScrollIndicator={isEnabled}
            alwaysBounceHorizontal={false}
            bounces={isEnabled}
            className="min-h-0 flex-1"
            contentContainerStyle={content}
            onScroll={onScroll}
            onContentSizeChange={onContentSizeChange}
            scrollEventThrottle={16}
        >
            {children(isEnabled ? scrollX : null)}
        </ScrollView>
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

interface TableListProps extends Omit<TableRowProps, 'item'> {
    rows: VisibleRow[]
    onNearEnd(): void
    isLoadingMore: boolean
}

function TableList({ rows, onNearEnd, isLoadingMore, ...row }: TableListProps) {
    const { columns, layout, oneLine, format, onToggleGroup, scrollX } = row
    const sticky = useMemo(() => stickyIndices(rows), [rows])
    const extraData = useMemo<TableData>(
        () => ({ columns, layout, oneLine, format, onToggleGroup, scrollX, rows }),
        [columns, layout, oneLine, format, onToggleGroup, scrollX, rows]
    )
    return (
        <>
            <HeadingRow
                columns={columns}
                layout={layout}
                oneLine={oneLine}
                format={format}
                scrollX={scrollX}
            />
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
                ListFooterComponent={<LoadingMore isVisible={isLoadingMore} />}
            />
        </>
    )
}

export interface ReportTableProps {
    columns: ResultColumn[]
    rows: VisibleRow[]
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
// asks for the next one too.
export function ReportTable(props: ReportTableProps) {
    const { rows, breakpoint, layouts, onToggleGroup, onNearEnd, isLoadingMore } = props
    const columns = useStable(props.columns)
    const format = useStable(props.format)
    const windowWidth = useWindowWidth()
    // The table's own width (a builder drawer narrows it): local layout state from onLayout.
    const [width, setWidth] = useState(windowWidth)
    const amountCount = Math.max(columns.length - 1, 0)
    const oneLine = useMemo(
        () => reportTableLayout({ breakpoint, amountCount }),
        [breakpoint, amountCount]
    )
    const fits = fitsOnOneLine({ breakpoint, amountCount, width })
    const layout = useMemo(
        () => tableLines({ columns, width, breakpoint, layouts, oneLineFits: fits }),
        [columns, width, breakpoint, layouts, fits]
    )
    if (columns.length === 0) return null
    const isOneLine = layout.kind === 'one-line'
    const list = (scrollX: Animated.Value | null) => (
        <TableList
            rows={rows}
            columns={columns}
            layout={layout}
            oneLine={oneLine}
            format={format}
            onToggleGroup={onToggleGroup}
            scrollX={scrollX}
            onNearEnd={onNearEnd}
            isLoadingMore={isLoadingMore}
        />
    )
    const notice = layout.kind === 'blocks' ? layout.notice : undefined
    return (
        <View
            testID="report-table"
            className="min-h-0 flex-1"
            onLayout={e => setWidth(e.nativeEvent.layout.width)}
        >
            <LayoutNotice text={notice} />
            <Sideways
                isEnabled={isOneLine && oneLine.scrollsSideways}
                minWidth={oneLine.labelMin + oneLine.amountContentMinWidth}
            >
                {list}
            </Sideways>
        </View>
    )
}
