import { type PickItem, PickMenu } from '@tinycld/core/ui/pick-menu'
import { X } from 'lucide-react-native'
import { Text, View } from 'react-native'
import type { ReportBuilder } from '../../hooks/useReportBuilder'
import { type ColumnDraft, columnWithFit } from '../../lib/builder/draft'
import type { ColumnFit } from '../../lib/spec'
import { FieldError, IconButton } from './parts'

// Design §3.6: a text column sets its narrowest width, whether long text wraps and
// its share of extra room; a figure is never cut, so it sets only extra room. Width,
// widest and % settings are kept as read (CLI or API) and not offered here.

const NARROWEST: { id: string; label: string; minWidth?: string }[] = [
    { id: 'auto', label: 'Automatic' },
    { id: 'whole', label: 'Whole value', minWidth: 'max-content' },
    { id: '6', label: '6 letters', minWidth: '6ch' },
    { id: '8', label: '8 letters', minWidth: '8ch' },
    { id: '12', label: '12 letters', minWidth: '12ch' },
    { id: '16', label: '16 letters', minWidth: '16ch' },
    { id: '24', label: '24 letters', minWidth: '24ch' },
]
const LONG_TEXT: { id: string; label: string; lines?: number }[] = [
    { id: 'one', label: 'One line, ends in …' },
    { id: '2', label: 'Wrap to 2 lines', lines: 2 },
    { id: '3', label: 'Wrap to 3 lines', lines: 3 },
]
// "Normal" is a text column's default; a figure's default is "None".
const EXTRA_ROOM = [
    { id: '0', label: 'None', grow: 0 },
    { id: '1', label: 'Normal', grow: 1 },
    { id: '2', label: 'Double', grow: 2 },
    { id: '3', label: 'Triple', grow: 3 },
]

function useColumnFit(
    builder: ReportBuilder,
    columns: ColumnDraft[],
    index: number,
    isFigure: boolean
) {
    const column = columns[index]
    const fit = column?.fit ?? {}
    const setFit = (next: ColumnFit) =>
        builder.setColumns(columns.map((c, i) => (i === index ? columnWithFit(c, next) : c)))
    const lines = fit.whiteSpace === 'normal' ? (fit.lineClamp ?? 2) : undefined
    // The measure's default share (column-measure.ts): text 1, a figure 0.
    const defaultGrow = isFigure ? 0 : 1
    const grow = fit.flexGrow ?? defaultGrow
    const narrowest: PickItem[] = NARROWEST.map(item => ({
        id: item.id,
        label: item.label,
        isSelected: item.minWidth === fit.minWidth,
        onSelect: () => setFit({ ...fit, minWidth: item.minWidth }),
    }))
    const longText: PickItem[] = LONG_TEXT.map(item => ({
        id: item.id,
        label: item.label,
        isSelected: item.lines === lines,
        onSelect: () =>
            setFit({
                ...fit,
                whiteSpace: item.lines ? 'normal' : undefined,
                lineClamp: item.lines,
            }),
    }))
    // The column's default share is not written.
    const extraRoom: PickItem[] = EXTRA_ROOM.map(item => ({
        id: item.id,
        label: item.label,
        isSelected: item.grow === grow,
        onSelect: () =>
            setFit({ ...fit, flexGrow: item.grow === defaultGrow ? undefined : item.grow }),
    }))
    const error = builder.form.getFieldState(`columns.${index}.fit`, builder.form.formState).error
    return {
        column,
        narrowest,
        longText,
        extraRoom,
        narrowestLabel:
            NARROWEST.find(item => item.minWidth === fit.minWidth)?.label ?? fit.minWidth ?? '',
        longTextLabel: LONG_TEXT.find(item => item.lines === lines)?.label ?? '',
        extraRoomLabel: EXTRA_ROOM.find(item => item.grow === grow)?.label ?? String(grow),
        hasFit: column?.fit !== undefined,
        error: error?.message,
        onClear: () => setFit({}),
    }
}

interface ColumnFitFieldProps {
    builder: ReportBuilder
    columns: ColumnDraft[]
    index: number
    /** The column shows numbers, amounts, percents, dates or yes/no values: never cut. */
    isFigure: boolean
}

export function ColumnFitField({ builder, columns, index, isFigure }: ColumnFitFieldProps) {
    const fit = useColumnFit(builder, columns, index, isFigure)
    if (!fit.column) return null
    const testID = `report-builder-column-${index}-fit`
    const name = fit.column.label
    return (
        <View className="gap-1">
            <View className="flex-row flex-wrap items-center gap-2">
                <TextFitMenus isVisible={!isFigure} fit={fit} name={name} testID={testID} />
                <NeverCut isVisible={isFigure} testID={testID} />
                <PickMenu
                    label={`Extra room: ${fit.extraRoomLabel}`}
                    purpose={`${name} extra room`}
                    items={fit.extraRoom}
                    testID={`${testID}-grow`}
                />
                <IconButton
                    label={`Clear ${name} fit`}
                    testID={`${testID}-clear`}
                    isVisible={fit.hasFit && fit.error !== undefined}
                    icon={X}
                    onPress={fit.onClear}
                />
            </View>
            <FieldError message={fit.error} testID={`${testID}-problem`} />
        </View>
    )
}

type ColumnFitState = ReturnType<typeof useColumnFit>

function TextFitMenus({
    isVisible,
    fit,
    name,
    testID,
}: {
    isVisible: boolean
    fit: ColumnFitState
    name: string
    testID: string
}) {
    if (!isVisible) return null
    return (
        <>
            <PickMenu
                label={`Narrowest: ${fit.narrowestLabel}`}
                purpose={`${name} narrowest width`}
                items={fit.narrowest}
                testID={`${testID}-min`}
            />
            <PickMenu
                label={`Long text: ${fit.longTextLabel}`}
                purpose={`${name} long text`}
                items={fit.longText}
                testID={`${testID}-wrap`}
            />
        </>
    )
}

function NeverCut({ isVisible, testID }: { isVisible: boolean; testID: string }) {
    if (!isVisible) return null
    return (
        <Text testID={`${testID}-never-cut`} className="text-xs text-muted-foreground">
            Never cut
        </Text>
    )
}
