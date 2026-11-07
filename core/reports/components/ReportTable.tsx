import { FlashList, type ListRenderItemInfo } from '@shopify/flash-list'
import { type Breakpoint, useWindowWidth } from '@tinycld/core/components/workspace/useBreakpoint'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { ArrowDown, ArrowUp, ArrowUpDown } from 'lucide-react-native'
import { useMemo, useRef, useState } from 'react'
import { ActivityIndicator, Platform, Pressable, Text, View } from 'react-native'
import { headingText, type NumberFormat } from '../lib/format'
import type { VisibleRow } from '../lib/rows'
import type { ViewSort } from '../lib/run-client'
import type { DisplayLayout } from '../lib/spec'
import { stickyIndices } from '../lib/sticky'
import { type Justify, type LineLayout, tableLines } from '../lib/table-lines'
import type { ResultColumn, ResultRow } from '../lib/types'
import {
    amountJustify,
    BlockView,
    cellBoxStyle,
    JUSTIFY_CONTENT,
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

/** The reader's sort the heading shows, and the press that changes it. */
interface HeadingSort {
    sort: ViewSort | null
    onSortColumn(col: string): void
}

// One object while the sort is the same: the parent gives a new sort object and a
// new press handler on every render. The press calls the latest handler, which
// closes over the reader's current view options.
function useHeadingSort(sort: ViewSort | null, onSortColumn: (col: string) => void): HeadingSort {
    const keptSort = useStable(sort)
    const onSortColumnRef = useRef(onSortColumn)
    onSortColumnRef.current = onSortColumn
    return useMemo(
        () => ({ sort: keptSort, onSortColumn: (col: string) => onSortColumnRef.current(col) }),
        [keptSort]
    )
}

const SORT_ICON_SIZE = 12
const SORT_ICONS = { asc: ArrowUp, desc: ArrowDown, none: ArrowUpDown } as const
type SortIconKind = keyof typeof SORT_ICONS

const sortIconOf = (sort: ViewSort | null, col: string): SortIconKind =>
    sort?.col === col ? sort.dir : 'none'

const SORT_STATE_TEXT: Record<SortIconKind, string | undefined> = {
    asc: 'ascending',
    desc: 'descending',
    none: undefined,
}

function SortIcon({ kind, isVisible }: { kind: SortIconKind; isVisible: boolean }) {
    const color = useThemeColor('muted-foreground')
    if (!isVisible) return null
    const Icon = SORT_ICONS[kind]
    return <Icon size={SORT_ICON_SIZE} color={color} />
}

function HeadingText({ text, justify }: { text: string; justify: Justify }) {
    return (
        <Text
            numberOfLines={1}
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
}

// R15: a column the run can sort by (ResultColumn.sortable) has a heading that is a
// button; each press cycles ascending → descending → the report's order. The icon
// sits outside the text, so the ellipsis never hides it; in a right-aligned column
// it comes first, so the text's right edge stays in line with the values. The
// direction is the button's value (aria-valuetext: native and web), so the name
// stays "Sort by <heading>". A running value's heading is plain text.
function HeadingContent({ column, text, justify, sorting }: HeadingContentProps) {
    if (!column.sortable) return <HeadingText text={text} justify={justify} />
    const kind = sortIconOf(sorting.sort, column.id)
    const iconFirst = justify === 'end'
    return (
        <Pressable
            testID={`report-sort-${column.id}`}
            accessibilityRole="button"
            accessibilityLabel={`Sort by ${text}`}
            aria-valuetext={SORT_STATE_TEXT[kind]}
            onPress={() => sorting.onSortColumn(column.id)}
            className="min-h-11 flex-1 flex-row items-center gap-1"
            style={{ justifyContent: JUSTIFY_CONTENT[justify] }}
        >
            <SortIcon kind={kind} isVisible={iconFirst} />
            <HeadingText text={text} justify={justify} />
            <SortIcon kind={kind} isVisible={!iconFirst} />
        </Pressable>
    )
}

function HeadingCell({
    column,
    justify,
    width,
    format,
    sorting,
}: {
    column: ResultColumn
    justify: Justify
    width?: number
    format: NumberFormat
    sorting: HeadingSort
}) {
    const text = headingText(column, format)
    return (
        <View
            testID={`report-heading-${column.id}`}
            className="justify-center px-3"
            style={cellBoxStyle(width)}
        >
            <HeadingContent column={column} text={text} justify={justify} sorting={sorting} />
        </View>
    )
}

// The label's heading sits over the label's text, which starts after the row's indent.
function LabelHeading({
    column,
    format,
    sorting,
}: {
    column: ResultColumn
    format: NumberFormat
    sorting: HeadingSort
}) {
    return (
        <View className="grow pl-1">
            <HeadingCell column={column} justify="start" format={format} sorting={sorting} />
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
        />
    ))
    return (
        <>
            <View style={oneLineLabelStyle(layout, label.id)}>
                <LabelHeading column={label} format={format} sorting={sorting} />
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
    const renderCell: RenderCell = (col, justify) => {
        const column = byId.get(col)
        if (!column) return null
        if (col === labelId)
            return <LabelHeading column={column} format={format} sorting={sorting} />
        return <HeadingCell column={column} justify={justify} format={format} sorting={sorting} />
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
    sorting: HeadingSort
}

function TableList({
    rows,
    onNearEnd,
    isLoadingMore,
    onRowWidth,
    sorting,
    ...row
}: TableListProps) {
    const { columns, layout, format, onToggleGroup } = row
    const sticky = useMemo(() => stickyIndices(rows), [rows])
    const extraData = useMemo<TableData>(
        () => ({ columns, layout, format, onToggleGroup, rows }),
        [columns, layout, format, onToggleGroup, rows]
    )
    // One element while the width setter stays, so FlashList keeps its header
    // rather than rebuilding it on every render.
    const header = useMemo(() => <RowWidthProbe onWidth={onRowWidth} />, [onRowWidth])
    return (
        <>
            <HeadingRow columns={columns} layout={layout} format={format} sorting={sorting} />
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
                ListHeaderComponent={header}
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
    /** The reader's sort (the view URL's `s=`); null: the report's own order. */
    sort: ViewSort | null
    onSortColumn(col: string): void
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
    const sorting = useHeadingSort(props.sort, props.onSortColumn)
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
                sorting={sorting}
            />
        </View>
    )
}
