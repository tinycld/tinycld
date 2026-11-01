import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { ChevronDown, ChevronRight } from 'lucide-react-native'
import { Pressable, Text, View } from 'react-native'
import { type CellDisplay, cellDisplay, type NumberFormat } from '../lib/format'
import type { VisibleRow } from '../lib/rows'
import type { ResultColumn, ResultRow, RowKind } from '../lib/types'
import { columnAlign, INDENT, type ReportTableLayout, ROW_HEIGHT } from './report-table-layout'

// The rule above a row; the label and amount stacks draw it alike, so it runs the full width.
const ROW_RULE: Record<RowKind, string> = {
    group: '',
    detail: '',
    subtotal: 'border-t border-border',
    computed: 'border-t border-border',
    total: 'border-t-2 border-foreground',
}

const ROW_WEIGHT: Record<RowKind, string> = {
    group: 'font-semibold',
    detail: '',
    subtotal: 'font-semibold',
    computed: 'font-semibold',
    total: 'font-semibold',
}

const CARET_SIZE = 14

function labelRowStyle(row: ResultRow) {
    return { height: ROW_HEIGHT, paddingLeft: INDENT + row.depth * INDENT }
}

function GroupCaret({ isOpen }: { isOpen: boolean }) {
    const color = useThemeColor('muted-foreground')
    const Icon = isOpen ? ChevronDown : ChevronRight
    return <Icon size={CARET_SIZE} color={color} />
}

function LabelText({ row }: { row: ResultRow }) {
    return (
        <Text
            numberOfLines={1}
            className={`min-w-0 flex-1 text-sm text-foreground ${ROW_WEIGHT[row.kind]}`}
        >
            {row.label}
        </Text>
    )
}

interface ReportRowLabelProps {
    item: VisibleRow
    onToggleGroup(row: ResultRow): void
}

// A role-button Pressable takes a click, Enter and Space on web and a tap on native.
function GroupToggleLabel({ item, onToggleGroup }: ReportRowLabelProps) {
    const { row } = item
    return (
        <Pressable
            testID={`report-row-${row.id}`}
            accessibilityRole="button"
            accessibilityLabel={row.label}
            aria-expanded={item.isOpen}
            onPress={() => onToggleGroup(row)}
            className="flex-row items-center gap-1 pr-3"
            style={labelRowStyle(row)}
        >
            <GroupCaret isOpen={item.isOpen} />
            <LabelText row={row} />
        </Pressable>
    )
}

export function ReportRowLabel({ item, onToggleGroup }: ReportRowLabelProps) {
    if (item.isToggle) return <GroupToggleLabel item={item} onToggleGroup={onToggleGroup} />
    const { row } = item
    return (
        <View
            testID={`report-row-${row.id}`}
            className={`flex-row items-center pr-3 ${ROW_RULE[row.kind]}`}
            style={labelRowStyle(row)}
        >
            <LabelText row={row} />
        </View>
    )
}

function cellTextClass(display: CellDisplay, align: 'left' | 'right', kind: RowKind): string {
    const alignClass = align === 'right' ? 'text-right tabular-nums' : 'text-left'
    const tone = display.isRed || display.isError ? 'text-danger' : 'text-foreground'
    return `text-sm ${alignClass} ${tone} ${ROW_WEIGHT[kind]}`
}

interface AmountCellProps {
    item: VisibleRow
    column: ResultColumn
    index: number
    width: number
    format: NumberFormat
}

function AmountCell({ item, column, index, width, format }: AmountCellProps) {
    const { row } = item
    const display = cellDisplay(column, row, item.cells[column.id], format)
    const align = columnAlign(column, index)
    return (
        <View
            testID={`report-cell-${column.id}-${row.id}`}
            className={`justify-center px-3 ${ROW_RULE[row.kind]}`}
            style={{ width, height: ROW_HEIGHT }}
        >
            <Text numberOfLines={1} className={cellTextClass(display, align, row.kind)}>
                {display.text}
            </Text>
        </View>
    )
}

interface ReportRowAmountsProps {
    item: VisibleRow
    columns: ResultColumn[]
    layout: ReportTableLayout
    format: NumberFormat
    onToggleGroup(row: ResultRow): void
}

// The label side is the row's one accessible toggle. This side takes the same press but
// is not a button and has no tab stop, so a closed group's figures stay readable.
export function ReportRowAmounts({
    item,
    columns,
    layout,
    format,
    onToggleGroup,
}: ReportRowAmountsProps) {
    const cells = columns.map((column, i) => (
        <AmountCell
            key={column.id}
            item={item}
            column={column}
            index={i + 1}
            width={layout.amountWidth}
            format={format}
        />
    ))
    if (!item.isToggle) return <View className="flex-row">{cells}</View>
    return (
        <Pressable
            onPress={() => onToggleGroup(item.row)}
            accessible={false}
            tabIndex={-1}
            className="flex-row"
        >
            {cells}
        </Pressable>
    )
}
