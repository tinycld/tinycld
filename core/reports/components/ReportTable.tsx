import {
    FlashList,
    type FlashListRef,
    type ListRenderItemInfo,
    type ViewToken,
} from '@shopify/flash-list'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { ArrowDown, ArrowUp, ArrowUpDown } from 'lucide-react-native'
import { type ReactNode, useCallback, useLayoutEffect, useMemo, useRef } from 'react'
import { ActivityIndicator, Animated, Platform, Pressable, Text, View } from 'react-native'
import { useScrollAwayTop } from '../hooks/useScrollAwayTop'
import { useStable } from '../hooks/useStable'
import { headingText, type NumberFormat } from '../lib/format'
import type { VisibleRow } from '../lib/rows'
import type { ViewSort } from '../lib/run-client'
import { stickyIndices } from '../lib/sticky'
import type { Justify, LineLayout } from '../lib/table-lines'
import type { ResultColumn, ResultRow } from '../lib/types'
import {
    amountJustify,
    cellBoxStyle,
    JUSTIFY_CONTENT,
    oneLineLabelStyle,
    type RenderCell,
    RootBlockView,
    StickyGroup,
    TableRow,
    type TableRowProps,
} from './ReportRow'
import { HEADING_LINE_HEIGHT, ROW_HEIGHT } from './report-table-layout'

const TEXT_ALIGN: Record<Justify, string> = {
    start: 'text-left',
    center: 'text-center',
    end: 'text-right',
}

/** The reader's sort the heading shows, and the press that changes it. */
interface HeadingSort {
    sort: ViewSort | null
    onSortColumn(col: string): void
}

// One object while the sort is the same: the parent gives a new sort object on
// every render, and a press handler that changes only with the sort.
function useHeadingSort(sort: ViewSort | null, onSortColumn: (col: string) => void): HeadingSort {
    const keptSort = useStable(sort)
    return useMemo(() => ({ sort: keptSort, onSortColumn }), [keptSort, onSortColumn])
}

const SORT_ICON_SIZE = 12
const SORT_ICONS = { asc: ArrowUp, desc: ArrowDown, none: ArrowUpDown } as const
type SortIconKind = keyof typeof SORT_ICONS

const sortIconOf = (sort: ViewSort | null, col: string): SortIconKind =>
    sort?.col === col ? sort.dir : 'none'

// The direction is in the button's name: a button has no other channel that both
// web and native readers announce (aria-valuetext is for range roles only).
const SORT_LABEL_SUFFIX: Record<SortIconKind, string> = {
    asc: ', sorted ascending',
    desc: ', sorted descending',
    none: '',
}

const sortLabel = (text: string, kind: SortIconKind) => `Sort by ${text}${SORT_LABEL_SUFFIX[kind]}`

function SortIcon({ kind, isVisible }: { kind: SortIconKind; isVisible: boolean }) {
    const color = useThemeColor('muted-foreground')
    if (!isVisible) return null
    const Icon = SORT_ICONS[kind]
    return <Icon size={SORT_ICON_SIZE} color={color} />
}

function HeadingText({ text, justify }: { text: string; justify: Justify }) {
    return (
        <Text
            numberOfLines={2}
            className={`shrink text-xs font-semibold uppercase text-muted-foreground ${TEXT_ALIGN[justify]}`}
        >
            {text}
        </Text>
    )
}

interface HeadingContentProps {
    column: ResultColumn
    text: string
    justify: Justify
    sorting: HeadingSort
    isPressable: boolean
}

// R15: a column the run can sort by (ResultColumn.sortable) has a heading that is a
// button on a one-line layout; each press cycles ascending → descending → the
// report's order. The icon sits outside the text, so the ellipsis never hides it; in
// a right-aligned column it comes first, so the text's right edge stays in line with
// the values. A running value's heading is plain text, and so is every heading when
// the rows wrap (D10): the Sort chip sorts then, and a sorted column keeps its arrow.
function HeadingContent({ column, text, justify, sorting, isPressable }: HeadingContentProps) {
    if (!column.sortable) return <HeadingText text={text} justify={justify} />
    const kind = sortIconOf(sorting.sort, column.id)
    const iconFirst = justify === 'end'
    // A plain heading shows only an arrow that says the column is sorted.
    const isShown = isPressable || kind !== 'none'
    const row = (
        <>
            <SortIcon kind={kind} isVisible={iconFirst && isShown} />
            <HeadingText text={text} justify={justify} />
            <SortIcon kind={kind} isVisible={!iconFirst && isShown} />
        </>
    )
    const style = { justifyContent: JUSTIFY_CONTENT[justify] }
    if (!isPressable) {
        return (
            <View className="flex-1 flex-row items-center gap-1" style={style}>
                {row}
            </View>
        )
    }
    return (
        <Pressable
            testID={`report-sort-${column.id}`}
            accessibilityRole="button"
            accessibilityLabel={sortLabel(text, kind)}
            onPress={() => sorting.onSortColumn(column.id)}
            className="min-h-11 flex-1 flex-row items-center gap-1"
            style={style}
        >
            {row}
        </Pressable>
    )
}

function HeadingCell({
    column,
    justify,
    width,
    format,
    sorting,
    isPressable,
    minHeight,
}: {
    column: ResultColumn
    justify: Justify
    width?: number
    format: NumberFormat
    sorting: HeadingSort
    isPressable: boolean
    minHeight: number
}) {
    const text = headingText(column, format)
    return (
        <View
            testID={`report-heading-${column.id}`}
            className="justify-center px-3"
            style={cellBoxStyle(width, minHeight)}
        >
            <HeadingContent
                column={column}
                text={text}
                justify={justify}
                sorting={sorting}
                isPressable={isPressable}
            />
        </View>
    )
}

// The label's heading sits over the label's text, which starts after the row's indent.
function LabelHeading({
    column,
    format,
    sorting,
    isPressable,
    minHeight,
}: {
    column: ResultColumn
    format: NumberFormat
    sorting: HeadingSort
    isPressable: boolean
    minHeight: number
}) {
    return (
        <View className="grow">
            <HeadingCell
                column={column}
                justify="start"
                format={format}
                sorting={sorting}
                isPressable={isPressable}
                minHeight={minHeight}
            />
        </View>
    )
}

interface HeadingRowProps {
    columns: ResultColumn[]
    layout: LineLayout
    format: NumberFormat
    sorting: HeadingSort
}

function OneLineHeading({ columns, layout, format, sorting }: HeadingRowProps) {
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
            sorting={sorting}
            isPressable
            minHeight={ROW_HEIGHT}
        />
    ))
    return (
        <>
            <View style={oneLineLabelStyle(layout, label.id)}>
                <LabelHeading
                    column={label}
                    format={format}
                    sorting={sorting}
                    isPressable
                    minHeight={ROW_HEIGHT}
                />
            </View>
            {cells}
        </>
    )
}

// The heading is drawn by the same blocks as the rows, so each heading sits over
// its values and wraps the same way.
function BlocksHeading({ columns, layout, format, sorting }: HeadingRowProps) {
    if (layout.kind !== 'blocks') return null
    const byId = new Map(columns.map(column => [column.id, column]))
    const labelId = columns[0]?.id
    const renderCell: RenderCell = (col, justify, minHeight) => {
        const column = byId.get(col)
        if (!column) return null
        if (col === labelId)
            return (
                <LabelHeading
                    column={column}
                    format={format}
                    sorting={sorting}
                    isPressable={false}
                    minHeight={minHeight}
                />
            )
        return (
            <HeadingCell
                column={column}
                justify={justify}
                format={format}
                sorting={sorting}
                isPressable={false}
                minHeight={minHeight}
            />
        )
    }
    return (
        <View className="flex-1">
            <RootBlockView
                root={layout.root}
                indent={layout.indent}
                renderCell={renderCell}
                firstHeight={HEADING_LINE_HEIGHT}
                restHeight={HEADING_LINE_HEIGHT}
            />
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

// D8, D12: the list ends in what comes next. While a later page is left the Total
// is held back, and this row stands in its place; after the last page, a short
// end row (a page footer is for exports only).
function TableEnd({ hasMore }: { hasMore: boolean }) {
    if (hasMore) {
        return (
            <View
                testID="report-loading-more"
                className="flex-row items-center justify-center gap-2 py-3"
            >
                <ActivityIndicator />
                <Text className="text-sm text-muted-foreground">Loading more rows…</Text>
            </View>
        )
    }
    return (
        <View testID="report-end" className="items-center py-3">
            <Text className="text-sm text-muted-foreground">End of report</Text>
        </View>
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

const rowType = (item: VisibleRow) => item.row.kind
const STICKY_CONFIG = { useNativeDriver: Platform.OS !== 'web' }
// A row counts as in view from its first pixel, at once: a fold pressed just after a
// scroll must read where the reader is now. The handler only writes a ref.
const VIEWABILITY_CONFIG = { minimumViewTime: 0 }

/** The index of the topmost row in view, or undefined when none is. */
function topIndexOf(tokens: readonly ViewToken<VisibleRow>[]): number | undefined {
    let top: number | undefined
    for (const { index } of tokens) {
        if (index !== null && (top === undefined || index < top)) top = index
    }
    return top
}

type ViewableChange = { viewableItems: ViewToken<VisibleRow>[] }

// Tells the run the index of the row at the top of the view, so a fold keeps the
// reader's group and loads no further (useReportRun). FlashList reports only when
// the indices in view change, so the run reads the row at that index when it needs
// it. One handler while onTopRowIndex stays.
function useTopRowReport(onTopRowIndex: ((index: number) => void) | undefined) {
    return useMemo(() => {
        if (!onTopRowIndex) return undefined
        return ({ viewableItems }: ViewableChange) => {
            const top = topIndexOf(viewableItems)
            if (top !== undefined) onTopRowIndex(top)
        }
    }, [onTopRowIndex])
}

// A row's width: the list's content, which on web is the list less the scroll
// bar's gutter (scrollbar-gutter: stable, any bar width, none for overlay bars),
// and on native the whole list. The heading row keeps the same gutter.
function RowWidthProbe({ onWidth }: { onWidth(width: number): void }) {
    return <View onLayout={e => onWidth(e.nativeEvent.layout.width)} />
}

interface TableListProps extends Omit<TableRowProps, 'item'> {
    rows: VisibleRow[]
    runKey: string
    onNearEnd(): void
    /** A later page is left: the list ends in "Loading more rows…", else in "End of report". */
    hasMore: boolean
    onRowWidth(width: number): void
    sorting: HeadingSort
    onTopRowIndex?(index: number): void
}

// A run with a new design, new params or a new sort is another list, so it starts at
// the top (F11). The list stays mounted (a remount would mount the phone's top area
// in its header again and take the focus off the chip that started the run), so the
// scroll goes back to the top instead. Row keys carry the run: FlashList keeps the
// row in view anchored when the rows change, which a live re-run of the same run
// needs, but a row of the earlier run (the same row in a new sort) must not pull
// the new run's scroll back down to it.
function useNewRunAtTop(runKey: string) {
    const listRef = useRef<FlashListRef<VisibleRow>>(null)
    const shownKey = useRef(runKey)
    useLayoutEffect(() => {
        if (shownKey.current === runKey) return
        shownKey.current = runKey
        listRef.current?.scrollToOffset({ offset: 0, animated: false })
    }, [runKey])
    const keyExtractor = useCallback((item: VisibleRow) => `${runKey}:${item.row.id}`, [runKey])
    return { listRef, keyExtractor }
}

// What both list variants give FlashList: the sticky rows, the rows' redraw signal,
// the top-row report, the list's ref and its row keys.
function useTableListData({
    rows,
    runKey,
    columns,
    layout,
    format,
    onToggleGroup,
    onTopRowIndex,
}: TableListProps) {
    const sticky = useMemo(() => stickyIndices(rows), [rows])
    const onViewableItemsChanged = useTopRowReport(onTopRowIndex)
    const extraData = useMemo<TableData>(
        () => ({ columns, layout, format, onToggleGroup, rows }),
        [columns, layout, format, onToggleGroup, rows]
    )
    const { listRef, keyExtractor } = useNewRunAtTop(runKey)
    return { sticky, extraData, onViewableItemsChanged, listRef, keyExtractor }
}

function FixedHeadingList(props: TableListProps) {
    const { rows, columns, layout, format, sorting, onRowWidth, onNearEnd, hasMore } = props
    const { sticky, extraData, onViewableItemsChanged, listRef, keyExtractor } =
        useTableListData(props)
    // One element while the width setter stays, so FlashList keeps its header
    // rather than rebuilding it on every render.
    const header = useMemo(() => <RowWidthProbe onWidth={onRowWidth} />, [onRowWidth])
    return (
        <>
            <HeadingRow columns={columns} layout={layout} format={format} sorting={sorting} />
            <FlashList
                ref={listRef}
                data={rows}
                renderItem={renderTableItem}
                keyExtractor={keyExtractor}
                getItemType={rowType}
                extraData={extraData}
                stickyHeaderIndices={sticky}
                stickyHeaderConfig={STICKY_CONFIG}
                onEndReached={onNearEnd}
                onEndReachedThreshold={0.5}
                onViewableItemsChanged={onViewableItemsChanged}
                viewabilityConfig={VIEWABILITY_CONFIG}
                className="web:[scrollbar-gutter:stable]"
                ListHeaderComponent={header}
                ListFooterComponent={<TableEnd hasMore={hasMore} />}
            />
        </>
    )
}

// D9: the phone's list. The top area is the list's header and scrolls with the rows;
// the heading row floats over the list (useScrollAwayTop). It comes first in the tree,
// so its sort buttons come before the rows in the reading order.
function ScrollAwayList(props: TableListProps & { top: ReactNode }) {
    const { rows, columns, layout, format, sorting, onRowWidth, onNearEnd, hasMore, top } = props
    const { sticky, extraData, onViewableItemsChanged, listRef, keyExtractor } =
        useTableListData(props)
    const away = useScrollAwayTop()
    const header = useMemo(
        () => (
            <>
                <View onLayout={away.onTopLayout}>{top}</View>
                <RowWidthProbe onWidth={onRowWidth} />
            </>
        ),
        [top, away.onTopLayout, onRowWidth]
    )
    // Wrapped headings take no presses (D10), so a drag that starts on them passes
    // to the list. One-line headings keep their sort presses.
    const headingTouches = layout.kind === 'blocks' ? 'none' : 'box-none'
    return (
        <View className="min-h-0 flex-1">
            <Animated.View
                style={away.headingStyle}
                onLayout={away.onHeadingLayout}
                pointerEvents={headingTouches}
            >
                <HeadingRow columns={columns} layout={layout} format={format} sorting={sorting} />
            </Animated.View>
            <FlashList
                ref={listRef}
                data={rows}
                renderItem={renderTableItem}
                keyExtractor={keyExtractor}
                getItemType={rowType}
                extraData={extraData}
                stickyHeaderIndices={sticky}
                stickyHeaderConfig={away.stickyConfig}
                onScroll={away.onScroll}
                onEndReached={onNearEnd}
                onEndReachedThreshold={0.5}
                onViewableItemsChanged={onViewableItemsChanged}
                viewabilityConfig={VIEWABILITY_CONFIG}
                className="web:[scrollbar-gutter:stable]"
                ListHeaderComponent={header}
                ListFooterComponent={<TableEnd hasMore={hasMore} />}
            />
        </View>
    )
}

function TableList({ top, ...props }: TableListProps & { top?: ReactNode }) {
    if (top === undefined) return <FixedHeadingList {...props} />
    return <ScrollAwayList {...props} top={top} />
}

export interface ReportTableProps {
    columns: ResultColumn[]
    rows: VisibleRow[]
    /** The design and params of the run whose rows are shown; a new value starts the list at the top. */
    runKey: string
    format: NumberFormat
    /** The lines of every row and the heading (useTableLayout). */
    layout: LineLayout
    /** False until the rows' width is measured: the table is drawn hidden until then. */
    isMeasured: boolean
    /** Told the width the rows really get (the list's content width). */
    onRowWidth(width: number): void
    onToggleGroup(row: ResultRow): void
    onNearEnd(): void
    /** A later page is left: the list ends in "Loading more rows…", else in "End of report". */
    hasMore: boolean
    /** The reader's sort (the view URL's `s=`); null: the report's own order. */
    sort: ViewSort | null
    onSortColumn(col: string): void
    /** Told the index (in `rows`) of the row at the top of the view whenever the indices in view change. */
    onTopRowIndex?(index: number): void
    /**
     * D9: the view's top area on a phone. Given, it scrolls away with the rows as the
     * list's header and the heading row stays at the top; not given, both stay put.
     */
    top?: ReactNode
}

// FlashList measures every row (wrapped rows have the heights their content
// gives them); onEndReached asks for the next page, through useReportRun's
// guard (hasNextPage and not already fetching). FlashList checks it after each
// commit and again when the rows change, so a first page shorter than the view
// asks for the next one too. The table never scrolls sideways (F10): its line is
// fitted to its width, narrowed, or wrapped (tableLines). The list ends in TableEnd:
// more rows on their way, or the end of the report.
export function ReportTable(props: ReportTableProps) {
    const { rows, runKey, layout, onToggleGroup, onNearEnd } = props
    const columns = useStable(props.columns)
    const format = useStable(props.format)
    const sorting = useHeadingSort(props.sort, props.onSortColumn)
    if (columns.length === 0) return null
    const visibility = props.isMeasured ? '' : 'opacity-0'
    return (
        <View testID="report-table" className={`min-h-0 flex-1 ${visibility}`}>
            <TableList
                runKey={runKey}
                top={props.top}
                rows={rows}
                columns={columns}
                layout={layout}
                format={format}
                onToggleGroup={onToggleGroup}
                onNearEnd={onNearEnd}
                hasMore={props.hasMore}
                onRowWidth={props.onRowWidth}
                sorting={sorting}
                onTopRowIndex={props.onTopRowIndex}
            />
        </View>
    )
}
