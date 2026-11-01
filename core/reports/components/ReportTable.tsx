import { type Breakpoint, useWindowHeight } from '@tinycld/core/components/workspace/useBreakpoint'
import { type RefObject, useRef } from 'react'
import {
    ActivityIndicator,
    type NativeScrollEvent,
    type NativeSyntheticEvent,
    ScrollView,
    type ScrollViewInstance,
    Text,
    View,
} from 'react-native'
import type { NumberFormat } from '../lib/format'
import type { VisibleRow } from '../lib/rows'
import type { ResultColumn, ResultRow } from '../lib/types'
import { ReportRowAmounts, ReportRowLabel } from './ReportRow'
import {
    columnAlign,
    type ReportTableLayout,
    ROW_HEIGHT,
    reportTableLayout,
} from './report-table-layout'
import { useRowWindow } from './use-row-window'

// Sized by its content, then shrunk (and scrolled) when the label column needs the room.
const AMOUNT_SCROLLER_STYLE = { flexGrow: 0, flexShrink: 1 }

function amountContentStyle(layout: ReportTableLayout) {
    return { width: '100%', minWidth: layout.amountContentMinWidth } as const
}

function HeadingCell({
    column,
    index,
    width,
}: {
    column: ResultColumn
    index: number
    width?: number
}) {
    const align = columnAlign(column, index) === 'right' ? 'text-right' : 'text-left'
    return (
        <View
            testID={`report-heading-${column.id}`}
            className="justify-center px-3"
            style={{ width, height: ROW_HEIGHT }}
        >
            <Text
                numberOfLines={1}
                className={`text-xs font-semibold uppercase text-muted-foreground ${align}`}
            >
                {column.label}
            </Text>
        </View>
    )
}

interface HeadingRowProps {
    labelColumn: ResultColumn
    amountColumns: ResultColumn[]
    layout: ReportTableLayout
    scrollRef: RefObject<ScrollViewInstance | null>
}

// Outside the rows' vertical scroller, so it stays visible; its amount labels sit in a
// scroller the body drives, so they move sideways in step with the amount columns.
function HeadingRow({ labelColumn, amountColumns, layout, scrollRef }: HeadingRowProps) {
    const amounts = amountColumns.map((column, i) => (
        <HeadingCell key={column.id} column={column} index={i + 1} width={layout.amountWidth} />
    ))
    return (
        <View
            testID="report-heading-row"
            className="flex-row border-b border-border bg-background web:overflow-y-hidden web:[scrollbar-gutter:stable]"
        >
            <View className="pl-1" style={layout.labelStyle}>
                <HeadingCell column={labelColumn} index={0} />
            </View>
            <ScrollView
                ref={scrollRef}
                horizontal
                scrollEnabled={false}
                showsHorizontalScrollIndicator={false}
                style={AMOUNT_SCROLLER_STYLE}
                contentContainerStyle={amountContentStyle(layout)}
            >
                {amounts}
            </ScrollView>
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

export interface ReportTableProps {
    columns: ResultColumn[]
    rows: VisibleRow[]
    format: NumberFormat
    breakpoint: Breakpoint
    onToggleGroup(row: ResultRow): void
    onNearEnd(): void
    isLoadingMore: boolean
}

// Two stacks in one vertical scroller: the label column stays put while the amount
// columns scroll sideways beside it, and both move together vertically.
export function ReportTable({
    columns,
    rows,
    format,
    breakpoint,
    onToggleGroup,
    onNearEnd,
    isLoadingMore,
}: ReportTableProps) {
    const [labelColumn, ...amountColumns] = columns
    const layout = reportTableLayout({ breakpoint, amountCount: amountColumns.length })
    const headingRef = useRef<ScrollViewInstance>(null)
    const rowWindow = useRowWindow({
        total: rows.length,
        rowHeight: ROW_HEIGHT,
        viewHeight: useWindowHeight(),
        onNearEnd,
    })
    if (!labelColumn) return null
    const shown = rows.slice(rowWindow.first, rowWindow.last)
    const labels = shown.map(item => (
        <ReportRowLabel key={item.row.id} item={item} onToggleGroup={onToggleGroup} />
    ))
    const amounts = shown.map(item => (
        <ReportRowAmounts
            key={item.row.id}
            item={item}
            columns={amountColumns}
            layout={layout}
            format={format}
            onToggleGroup={onToggleGroup}
        />
    ))
    const topSpacer = { height: rowWindow.topSpace }
    const bottomSpacer = { height: rowWindow.bottomSpace }
    const onAmountsScroll = (event: NativeSyntheticEvent<NativeScrollEvent>) =>
        headingRef.current?.scrollTo({ x: event.nativeEvent.contentOffset.x, animated: false })
    return (
        <View testID="report-table" className="min-h-0 flex-1">
            <HeadingRow
                labelColumn={labelColumn}
                amountColumns={amountColumns}
                layout={layout}
                scrollRef={headingRef}
            />
            <ScrollView
                className="flex-1 web:[scrollbar-gutter:stable]"
                onScroll={rowWindow.onScroll}
                onLayout={rowWindow.onLayout}
                onContentSizeChange={rowWindow.onContentSizeChange}
                scrollEventThrottle={16}
            >
                <View className="flex-row">
                    <View style={layout.labelStyle}>
                        <View style={topSpacer} />
                        {labels}
                        <View style={bottomSpacer} />
                    </View>
                    <ScrollView
                        horizontal
                        scrollEnabled={layout.scrollsSideways}
                        style={AMOUNT_SCROLLER_STYLE}
                        contentContainerStyle={amountContentStyle(layout)}
                        onScroll={onAmountsScroll}
                        scrollEventThrottle={16}
                    >
                        <View className="flex-1">
                            <View style={topSpacer} />
                            {amounts}
                            <View style={bottomSpacer} />
                        </View>
                    </ScrollView>
                </View>
                <LoadingMore isVisible={isLoadingMore} />
            </ScrollView>
        </View>
    )
}
