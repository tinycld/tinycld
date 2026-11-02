import { FlashList, type FlashListRef, type ListRenderItemInfo } from '@shopify/flash-list'
import { type Breakpoint, useWindowWidth } from '@tinycld/core/components/workspace/useBreakpoint'
import { type ReactNode, useMemo, useRef, useState } from 'react'
import {
    ActivityIndicator,
    Animated,
    type NativeScrollEvent,
    type NativeSyntheticEvent,
    Platform,
    ScrollView,
    type ScrollViewInstance,
    Text,
    View,
} from 'react-native'
import { headingText, type NumberFormat } from '../lib/format'
import type { VisibleRow } from '../lib/rows'
import type { DisplayLayout } from '../lib/spec'
import { groupChain, stickyIndices, stickyPush } from '../lib/sticky'
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
// frozen label column. It stays mounted when sideways scroll turns off (a
// narrower width), so the list keeps its vertical position; its content is then
// the view's width and its offset goes back to 0.
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
            scrollEnabled={isEnabled}
            showsHorizontalScrollIndicator={isEnabled}
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

// FlashList pushes the stuck copy by the height it measured for the stuck row;
// the copy is as tall as its stack of groups. This follows the list's scroll and
// moves the copy by the difference (lib/sticky.ts stickyPush), so it is pushed
// by its own height.
function useStickyPush(rows: VisibleRow[], sticky: number[]) {
    const listRef = useRef<FlashListRef<VisibleRow>>(null)
    const push = useRef(new Animated.Value(0)).current
    const stuck = useRef({ index: -1, height: 0 })
    const onScroll = (event: NativeSyntheticEvent<NativeScrollEvent>) => {
        const list = listRef.current
        if (!list) return
        const next = sticky.find(index => index > stuck.current.index)
        const nextLayout = next === undefined ? undefined : list.getLayout(next)
        push.setValue(
            stickyPush({
                offset: event.nativeEvent.contentOffset.y,
                nextY: nextLayout ? nextLayout.y + list.getFirstItemOffset() : undefined,
                stackHeight: stuck.current.height,
                itemHeight: list.getLayout(stuck.current.index)?.height ?? 0,
            })
        )
    }
    return {
        listRef,
        push,
        onScroll,
        // A boundary with no open group above draws no copy, so it has no height.
        onChangeStickyIndex: (index: number) => {
            const hasCopy = index >= 0 && groupChain(rows, index).length > 0
            stuck.current = { index, height: hasCopy ? stuck.current.height : 0 }
        },
        onHeight: (height: number) => {
            stuck.current = { ...stuck.current, height }
        },
    }
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
    const oneLine = reportTableLayout({ breakpoint, amountCount })
    const fits = fitsOnOneLine({ breakpoint, amountCount, width })
    const layout = useMemo(
        () => tableLines({ columns, width, breakpoint, layouts, oneLineFits: fits }),
        [columns, width, breakpoint, layouts, fits]
    )
    const extraData = useMemo(() => ({ layout, format }), [layout, format])
    const sticky = stickyIndices(rows)
    const stickyPushing = useStickyPush(rows, sticky)
    if (columns.length === 0) return null
    const isOneLine = layout.kind === 'one-line'
    const tableRow = (scrollX: Animated.Value | null) => ({
        columns,
        layout,
        oneLine,
        format,
        onToggleGroup,
        scrollX,
    })
    const list = (scrollX: Animated.Value | null) => {
        const renderItem = ({ item, index, target }: ListRenderItemInfo<VisibleRow>) => {
            if (target !== 'StickyHeader') return <TableRow {...tableRow(scrollX)} item={item} />
            return (
                <Animated.View style={{ transform: [{ translateY: stickyPushing.push }] }}>
                    <StickyGroup
                        {...tableRow(scrollX)}
                        rows={rows}
                        index={index}
                        onHeight={stickyPushing.onHeight}
                    />
                </Animated.View>
            )
        }
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
                    ref={stickyPushing.listRef}
                    data={rows}
                    renderItem={renderItem}
                    keyExtractor={item => item.row.id}
                    getItemType={item => item.row.kind}
                    extraData={extraData}
                    stickyHeaderIndices={sticky}
                    stickyHeaderConfig={{ useNativeDriver: Platform.OS !== 'web' }}
                    onChangeStickyIndex={stickyPushing.onChangeStickyIndex}
                    onScroll={stickyPushing.onScroll}
                    onEndReached={onNearEnd}
                    onEndReachedThreshold={0.5}
                    className="web:[scrollbar-gutter:stable]"
                    ListFooterComponent={<LoadingMore isVisible={isLoadingMore} />}
                />
            </>
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
            >
                {list}
            </Sideways>
        </View>
    )
}
