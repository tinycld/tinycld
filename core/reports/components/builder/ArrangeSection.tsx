import { useBreakpoint } from '@tinycld/core/components/workspace/useBreakpoint'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { useWatch } from '@tinycld/core/ui/form'
import { type PickItem, PickMenu } from '@tinycld/core/ui/pick-menu'
import { Switch } from '@tinycld/core/ui/switch'
import { Trash2 } from 'lucide-react-native'
import { Text, View } from 'react-native'
import type { ReportBuilder } from '../../hooks/useReportBuilder'
import { type ColumnDraft, displayIssues } from '../../lib/builder/draft'
import type { DisplayCell, DisplayLayout, DisplayWidth } from '../../lib/spec'
import {
    defaultJustify,
    extent,
    MAX_LINES,
    MAX_TRACKS,
    startingLayout,
} from '../../lib/table-lines'
import type { Problem, ResultColumn } from '../../lib/types'
import { useReportUIStore } from '../../stores/report-ui-store'
import { BuilderSection } from './BuilderSection'
import { IconButton } from './parts'

// E14: per width, the author places each column on a line of equal tracks; a
// column left out is not shown at that width. Off, the width is laid out
// automatically. Placement follows the engine's rules (lib/table-lines.ts), and
// what it refuses shows on the cell it names before Save.

const WIDTHS: { value: DisplayWidth; label: string }[] = [
    { value: 'mobile', label: 'Phone' },
    { value: 'tablet', label: 'Tablet' },
    { value: 'desktop', label: 'Desktop' },
]
const WIDTH_NAMES: Record<DisplayWidth, string> = {
    mobile: 'Phone',
    tablet: 'Tablet',
    desktop: 'Desktop',
}
const JUSTIFY_NAMES = { start: 'Start', center: 'Center', end: 'End' } as const

const counting = (from: number, to: number) =>
    Array.from({ length: to - from + 1 }, (_, i) => from + i)

type Issue = ReturnType<typeof displayIssues>[number]

/** A cell narrowed to `tracks`: a wider span shrinks to it, and its start moves in to fit. */
function fitCell(cell: DisplayCell, tracks: number): DisplayCell {
    const span = cell.span === undefined ? undefined : Math.min(cell.span, tracks)
    const start =
        cell.start === undefined ? undefined : Math.min(cell.start, tracks - (span ?? 1) + 1)
    return { ...cell, span, start }
}

/** The line below every placed cell, for a column shown again. */
function nextLine(cells: readonly DisplayCell[]): number {
    const last = Math.max(0, ...cells.map(c => c.line + extent(c).lines - 1))
    return Math.min(MAX_LINES, last + 1)
}

/**
 * The Arrange section's state. `resultColumns` are the preview's columns: a cell
 * that sets no alignment aligns as the table draws it (lib/table-lines.ts).
 */
function useArrange(builder: ReportBuilder, resultColumns: readonly ResultColumn[]) {
    const { control } = builder.form
    const display = useWatch({ control, name: 'display' })
    const columns = useWatch({ control, name: 'columns' })
    const width = useReportUIStore(s => s.arrangeWidth)
    const setWidth = useReportUIStore(s => s.setArrangeWidth)
    const breakpoint = useBreakpoint()
    const index = display.layouts.findIndex(l => l.width === width)
    const layout = display.layouts[index]
    const issues = displayIssues(
        display,
        columns.map(c => c.id)
    ).filter(issue => issue.path[2] === index)
    const setLayout = (next: DisplayLayout) =>
        builder.setDisplay({
            layouts:
                index < 0
                    ? [...display.layouts, next]
                    : display.layouts.map((l, i) => (i === index ? next : l)),
        })
    const setCells = (cells: DisplayCell[]) => {
        if (layout) setLayout({ ...layout, cells })
    }
    return {
        width,
        setWidth,
        layout,
        columns,
        issues,
        defaultJustify: (col: string) => defaultJustify(resultColumns, col),
        // The preview draws the arrangement of the width it is drawn at.
        hint:
            width === breakpoint
                ? null
                : `The preview is drawn at ${WIDTH_NAMES[breakpoint]} width; this arrangement shows at ${WIDTH_NAMES[width]} width.`,
        onSwitch: (isOn: boolean) => {
            if (isOn) {
                setLayout(startingLayout(width, columns))
                return
            }
            builder.setDisplay({ layouts: display.layouts.filter((_, i) => i !== index) })
        },
        setTracks: (tracks: number) => {
            if (layout)
                setLayout({ ...layout, tracks, cells: layout.cells.map(c => fitCell(c, tracks)) })
        },
        setCell: (cellIndex: number, patch: Partial<DisplayCell>) =>
            setCells(layout?.cells.map((c, i) => (i === cellIndex ? { ...c, ...patch } : c)) ?? []),
        showColumn: (col: string, isShown: boolean) => {
            const cells = layout?.cells ?? []
            setCells(
                isShown
                    ? [...cells, { col, line: nextLine(cells) }]
                    : cells.filter(c => c.col !== col)
            )
        },
    }
}

type Arrange = ReturnType<typeof useArrange>

function WidthTab({ item, arrange }: { item: (typeof WIDTHS)[number]; arrange: Arrange }) {
    const variant = item.value === arrange.width ? 'default' : 'outline'
    return (
        <Button
            testID={`report-builder-arrange-width-${item.value}`}
            variant={variant}
            size="sm"
            accessibilityRole="tab"
            aria-selected={item.value === arrange.width}
            onPress={() => arrange.setWidth(item.value)}
        >
            <ButtonText>{item.label}</ButtonText>
        </Button>
    )
}

function Hint({ text }: { text: string | null }) {
    if (!text) return null
    return (
        <Text testID="report-builder-arrange-hint" className="text-xs text-muted-foreground">
            {text}
        </Text>
    )
}

function IssueLines({ issues, testID }: { issues: Issue[]; testID: string }) {
    if (issues.length === 0) return null
    const lines = issues.map(issue => (
        <Text key={issue.path.join('.')} className="text-xs text-danger">
            {issue.message}
        </Text>
    ))
    return (
        <View testID={testID} accessibilityRole="alert" className="gap-1">
            {lines}
        </View>
    )
}

interface CellMenusProps {
    column: ColumnDraft
    cell: DisplayCell
    cellIndex: number
    tracks: number
    arrange: Arrange
}

// Each choice writes one field of the cell; Start "Auto" takes the first free tracks.
function CellMenus({ column, cell, cellIndex, tracks, arrange }: CellMenusProps) {
    const testID = `report-builder-arrange-${column.id}`
    const set = (patch: Partial<DisplayCell>) => arrange.setCell(cellIndex, patch)
    const numbers = (values: number[], current: number, field: 'line' | 'span' | 'lines') =>
        values.map(
            (n): PickItem => ({
                id: String(n),
                label: String(n),
                isSelected: n === current,
                onSelect: () => set({ [field]: n }),
            })
        )
    const starts: PickItem[] = [
        {
            id: 'auto',
            label: 'Auto',
            isSelected: cell.start === undefined,
            onSelect: () => set({ start: undefined }),
        },
        ...counting(1, tracks).map(n => ({
            id: String(n),
            label: String(n),
            isSelected: n === cell.start,
            onSelect: () => set({ start: n }),
        })),
    ]
    // A cell that sets no alignment shows the one the table gives it.
    const justify = cell.justify ?? arrange.defaultJustify(column.id)
    const aligns = (['start', 'center', 'end'] as const).map(value => ({
        id: value,
        label: JUSTIFY_NAMES[value],
        isSelected: value === justify,
        onSelect: () => set({ justify: value }),
    }))
    const { span, lines } = extent(cell)
    const startLabel = cell.start === undefined ? 'Auto' : String(cell.start)
    return (
        <View className="flex-row flex-wrap gap-2">
            <PickMenu
                label={`Line ${cell.line}`}
                purpose={`${column.label} line`}
                items={numbers(counting(1, MAX_LINES), cell.line, 'line')}
                testID={`${testID}-line`}
            />
            <PickMenu
                label={`Start ${startLabel}`}
                purpose={`${column.label} start`}
                items={starts}
                testID={`${testID}-start`}
            />
            <PickMenu
                label={`Span ${span}`}
                purpose={`${column.label} span`}
                items={numbers(counting(1, tracks), span, 'span')}
                testID={`${testID}-span`}
            />
            <PickMenu
                label={`Lines ${lines}`}
                purpose={`${column.label} lines`}
                items={numbers(counting(1, MAX_LINES), lines, 'lines')}
                testID={`${testID}-lines`}
            />
            <PickMenu
                label={`Align ${JUSTIFY_NAMES[justify].toLowerCase()}`}
                purpose={`${column.label} align`}
                items={aligns}
                testID={`${testID}-justify`}
            />
        </View>
    )
}

function ShownCell(props: CellMenusProps & { isVisible: boolean; issues: Issue[] }) {
    if (!props.isVisible) return null
    return (
        <>
            <CellMenus {...props} />
            <IssueLines
                issues={props.issues}
                testID={`report-builder-arrange-${props.column.id}-problem`}
            />
        </>
    )
}

const cellIssues = (issues: readonly Issue[], cellIndex: number) =>
    issues.filter(issue => issue.path[3] === 'cells' && issue.path[4] === cellIndex)

function ColumnRow({
    column,
    layout,
    arrange,
}: {
    column: ColumnDraft
    layout: DisplayLayout
    arrange: Arrange
}) {
    const cellIndex = layout.cells.findIndex(c => c.col === column.id)
    const cell = layout.cells[cellIndex]
    const testID = `report-builder-arrange-${column.id}`
    return (
        <View
            testID={`report-builder-arrange-col-${column.id}`}
            className="gap-2 rounded-lg border border-border p-2"
        >
            <View className="flex-row items-center justify-between gap-2">
                <Text className="flex-1 text-sm text-foreground">{column.label}</Text>
                <Text className="text-xs text-muted-foreground">Show</Text>
                <Switch
                    value={cell !== undefined}
                    onValueChange={isShown => arrange.showColumn(column.id, isShown)}
                    accessibilityLabel={`Show ${column.label}`}
                    testID={`${testID}-show`}
                />
            </View>
            <ShownCell
                isVisible={cell !== undefined}
                column={column}
                cell={cell ?? { col: column.id, line: 1 }}
                cellIndex={cellIndex}
                tracks={layout.tracks}
                arrange={arrange}
                issues={cellIssues(arrange.issues, cellIndex)}
            />
        </View>
    )
}

// A column the arrangement places but the report no longer has: the engine
// refuses its cells, so they can only be removed.
function StrayColumn({ col, arrange }: { col: string; arrange: Arrange }) {
    return (
        <View className="flex-row items-center gap-2 rounded-lg border border-border p-2">
            <Text className="flex-1 text-sm text-danger">{`${col} is not in the report`}</Text>
            <IconButton
                label="Remove"
                testID={`report-builder-arrange-stray-${col}-remove`}
                isVisible
                icon={Trash2}
                onPress={() => arrange.showColumn(col, false)}
            />
        </View>
    )
}

function ArrangeFields({ arrange }: { arrange: Arrange }) {
    const { layout, columns } = arrange
    if (!layout) return null
    const ids = new Set(columns.map(c => c.id))
    const ownIndex = new Set(columns.map(c => layout.cells.findIndex(cell => cell.col === c.id)))
    const tracks = counting(1, MAX_TRACKS).map(n => ({
        id: String(n),
        label: String(n),
        isSelected: n === layout.tracks,
        onSelect: () => arrange.setTracks(n),
    }))
    // Problems no column row shows: the layout's own, and those of a stray or repeated cell.
    const general = arrange.issues.filter(
        issue => !(issue.path[3] === 'cells' && ownIndex.has(Number(issue.path[4])))
    )
    const rows = columns.map(column => (
        <ColumnRow key={column.id} column={column} layout={layout} arrange={arrange} />
    ))
    const strayCols = new Set(layout.cells.map(cell => cell.col).filter(col => !ids.has(col)))
    const strays = [...strayCols].map(col => <StrayColumn key={col} col={col} arrange={arrange} />)
    return (
        <View className="gap-2">
            <View className="flex-row items-center gap-2">
                <Text className="flex-1 text-sm text-foreground">Columns per line</Text>
                <PickMenu
                    label={String(layout.tracks)}
                    purpose="Columns per line"
                    items={tracks}
                    testID="report-builder-arrange-tracks"
                />
            </View>
            <IssueLines issues={general} testID="report-builder-arrange-problems" />
            {rows}
            {strays}
        </View>
    )
}

// F10: with no arrangement, a width fits its columns on one line, cutting long
// text short, and wraps only when they still do not fit.
function AutoCaption({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return (
        <Text testID="report-builder-arrange-auto" className="text-xs text-muted-foreground">
            Off: the columns fit on one line, with long text cut short, or wrap when they do not
            fit.
        </Text>
    )
}

export function ArrangeSection({
    builder,
    problems,
    resultColumns,
}: {
    builder: ReportBuilder
    problems: Problem[]
    /** The preview's columns. */
    resultColumns: readonly ResultColumn[]
}) {
    const arrange = useArrange(builder, resultColumns)
    const tabs = WIDTHS.map(item => <WidthTab key={item.value} item={item} arrange={arrange} />)
    return (
        <BuilderSection title="Arrange" section="arrange" problems={problems}>
            <View accessibilityRole="tablist" className="flex-row gap-2">
                {tabs}
            </View>
            <View className="flex-row items-center justify-between gap-2">
                <Text className="flex-1 text-sm text-foreground">
                    Arrange columns for this width
                </Text>
                <Switch
                    value={arrange.layout !== undefined}
                    onValueChange={arrange.onSwitch}
                    accessibilityLabel="Arrange columns for this width"
                    testID="report-builder-arrange-on"
                />
            </View>
            <AutoCaption isVisible={arrange.layout === undefined} />
            <Hint text={arrange.hint} />
            <ArrangeFields arrange={arrange} />
        </BuilderSection>
    )
}
