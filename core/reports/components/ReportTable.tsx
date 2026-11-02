import { FlashList, type ListRenderItemInfo } from '@shopify/flash-list'
import { type Breakpoint, useWindowWidth } from '@tinycld/core/components/workspace/useBreakpoint'
import { type ReactNode, useRef, useState } from 'react'
import { ActivityIndicator, Animated, Platform, ScrollView, Text, View } from 'react-native'
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
} from './ReportRow'
import { fitsOnOneLine, type ReportTableLayout, reportTableLayout } from './report-table-layout'

const TEXT_ALIGN: Record<Justify, string> = {
    start: 'text-left',
    center: 'text-center',
    end: 'text-right',
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

interface HeadingRowProps {
    columns: ResultColumn[]
    layout: LineLayout
    oneLine: ReportTableLayout
    format: NumberFormat
    scrollX: Animated.Value
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
                <View className="pl-1">
                    <HeadingCell column={label} justify="start" format={format} />
                </View>
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
    const renderCell: RenderCell = (col, justify) => {
        const column = byId.get(col)
        if (!column) return null
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
    scrollX: Animated.Value
    children: ReactNode
}

// A desktop table wider than its view scrolls sideways as a whole: the heading
// row and the list sit in one horizontal scroller, and the list scrolls
// vertically inside it. The sideways offset drives the frozen label column.
function Sideways({ isEnabled, minWidth, scrollX, children }: SidewaysProps) {
    if (!isEnabled) return <View className="min-h-0 flex-1">{children}</View>
    const onScroll = Animated.event([{ nativeEvent: { contentOffset: { x: scrollX } } }], {
        useNativeDriver: false,
    })
    return (
        <ScrollView
            horizontal
            className="min-h-0 flex-1"
            contentContainerStyle={{ width: '100%', minWidth, flexDirection: 'column' }}
            onScroll={onScroll}
            scrollEventThrottle={16}
        >
            {children}
        </ScrollView>
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
export function ReportTable({
    columns,
    rows,
    format,
    breakpoint,
    layouts,
    onToggleGroup,
    onNearEnd,
    isLoadingMore,
}: ReportTableProps) {
    const windowWidth = useWindowWidth()
    // The table's own width (a builder drawer narrows it): local layout state from onLayout.
    const [width, setWidth] = useState(windowWidth)
    const scrollX = useRef(new Animated.Value(0)).current
    const amountCount = Math.max(columns.length - 1, 0)
    const oneLine = reportTableLayout({ breakpoint, amountCount })
    const layout = tableLines({
        columns,
        width,
        breakpoint,
        layouts,
        oneLineFits: fitsOnOneLine({ breakpoint, amountCount, width }),
    })
    if (columns.length === 0) return null
    const isOneLine = layout.kind === 'one-line'
    const renderItem = ({ item, index, target }: ListRenderItemInfo<VisibleRow>) => {
        if (target === 'StickyHeader') {
            return (
                <StickyGroup
                    rows={rows}
                    index={index}
                    format={format}
                    onToggleGroup={onToggleGroup}
                    oneLine={oneLine}
                    isOneLine={isOneLine}
                    scrollX={scrollX}
                />
            )
        }
        return (
            <TableRow
                item={item}
                columns={columns}
                layout={layout}
                oneLine={oneLine}
                format={format}
                onToggleGroup={onToggleGroup}
                scrollX={scrollX}
            />
        )
    }
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
                scrollX={scrollX}
            >
                <HeadingRow
                    columns={columns}
                    layout={layout}
                    oneLine={oneLine}
                    format={format}
                    scrollX={scrollX}
                />
                <FlashList
                    data={rows}
                    renderItem={renderItem}
                    keyExtractor={item => item.row.id}
                    getItemType={item => item.row.kind}
                    extraData={layout}
                    stickyHeaderIndices={stickyIndices(rows)}
                    stickyHeaderConfig={{ useNativeDriver: Platform.OS !== 'web' }}
                    onEndReached={onNearEnd}
                    onEndReachedThreshold={0.5}
                    className="web:[scrollbar-gutter:stable]"
                    ListFooterComponent={<LoadingMore isVisible={isLoadingMore} />}
                />
            </Sideways>
        </View>
    )
}
