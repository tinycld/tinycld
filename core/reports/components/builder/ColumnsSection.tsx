import { SelectInput, TextInput, useWatch } from '@tinycld/core/ui/form'
import { ArrowDown, ArrowUp, Trash2 } from 'lucide-react-native'
import { Text, View } from 'react-native'
import type { ReportBuilder } from '../../hooks/useReportBuilder'
import {
    AGGREGATE_LABELS,
    aggregatesFor,
    columnHint,
    defaultAggregate,
    describePath,
    nextColumnId,
    pathLabel,
} from '../../lib/builder/columns'
import type { ColumnDraft } from '../../lib/builder/draft'
import type { SpecHop } from '../../lib/spec'
import type { Aggregate, Problem } from '../../lib/types'
import { BuilderSection } from './BuilderSection'
import { ColumnPicker } from './ColumnPicker'
import { IconButton } from './parts'

const FORMAT_OPTIONS = [
    { value: '', label: 'Number' },
    { value: 'percent', label: 'Percent' },
]
const FORMULA_AGGREGATES: Aggregate[] = ['sum', 'min', 'max', 'avg']

export function moved<T>(items: readonly T[], from: number, to: number): T[] {
    const next = [...items]
    const [item] = next.splice(from, 1)
    if (item !== undefined) next.splice(to, 0, item)
    return next
}

function offeredAggregates(builder: ReportBuilder, column: ColumnDraft): Aggregate[] {
    if (column.kind === 'formula') return FORMULA_AGGREGATES
    const info = describePath(builder.catalog, builder.base.key, column.path)
    return info ? aggregatesFor(info.field, info.viaToMany) : []
}

function aggregateOptions(builder: ReportBuilder, column: ColumnDraft) {
    const offered = offeredAggregates(builder, column).map(a => ({
        value: a,
        label: AGGREGATE_LABELS[a],
    }))
    return [{ value: '', label: 'None' }, ...offered]
}

function columnSummary(builder: ReportBuilder, column: ColumnDraft): string {
    if (column.kind === 'formula') return 'Formula'
    const info = describePath(builder.catalog, builder.base.key, column.path)
    if (!info) return 'A field that is no longer offered'
    if (info.mode === 'nest') return `${pathLabel(info)} · child rows`
    return info.mode === 'aggregate' ? `${pathLabel(info)} · total` : pathLabel(info)
}

function isNumberColumn(builder: ReportBuilder, column: ColumnDraft): boolean {
    if (column.kind === 'formula') return true
    return describePath(builder.catalog, builder.base.key, column.path)?.field.type === 'number'
}

interface ColumnRowProps {
    builder: ReportBuilder
    columns: ColumnDraft[]
    index: number
}

function ColumnRow({ builder, columns, index }: ColumnRowProps) {
    const column = columns[index]
    if (!column) return null
    const { control } = builder.form
    const testID = `report-builder-column-${index}`
    const last = columns.length - 1
    return (
        <View testID={testID} className="gap-1 rounded-lg border border-border p-2">
            <View className="flex-row items-center">
                <Text numberOfLines={1} className="min-w-0 flex-1 text-xs text-muted-foreground">
                    {columnSummary(builder, column)}
                </Text>
                <IconButton
                    label={`Move ${column.label} up`}
                    testID={`${testID}-up`}
                    isVisible={index > 0}
                    icon={ArrowUp}
                    onPress={() => builder.setColumns(moved(columns, index, index - 1))}
                />
                <IconButton
                    label={`Move ${column.label} down`}
                    testID={`${testID}-down`}
                    isVisible={index < last}
                    icon={ArrowDown}
                    onPress={() => builder.setColumns(moved(columns, index, index + 1))}
                />
                <IconButton
                    label={`Remove ${column.label}`}
                    testID={`${testID}-remove`}
                    isVisible={columns.length > 1}
                    icon={Trash2}
                    onPress={() => builder.setColumns(columns.filter((_, i) => i !== index))}
                />
            </View>
            <TextInput
                name={`columns.${index}.label`}
                control={control}
                label="Label"
                testID={`${testID}-label`}
                maxLength={200}
            />
            <SelectInput
                name={`columns.${index}.aggregate`}
                control={control}
                label="Total"
                options={aggregateOptions(builder, column)}
                testID={`${testID}-aggregate`}
            />
            <FormatField builder={builder} column={column} index={index} />
            <ColumnFormula builder={builder} columns={columns} index={index} />
        </View>
    )
}

function FormatField({
    builder,
    column,
    index,
}: {
    builder: ReportBuilder
    column: ColumnDraft
    index: number
}) {
    if (!isNumberColumn(builder, column)) return null
    return (
        <SelectInput
            name={`columns.${index}.format`}
            control={builder.form.control}
            label="Show as"
            options={FORMAT_OPTIONS}
            testID={`report-builder-column-${index}-format`}
        />
    )
}

// A formula column's editor is drawn here once the builder edits formulas.
function ColumnFormula(_props: ColumnRowProps) {
    return null
}

export function ColumnsSection({
    builder,
    problems,
}: {
    builder: ReportBuilder
    problems: Problem[]
}) {
    const columns = useWatch({ control: builder.form.control, name: 'columns' })
    const onPick = (path: SpecHop[]) => {
        const info = describePath(builder.catalog, builder.base.key, path)
        if (!info) return
        const column: ColumnDraft = {
            kind: 'path',
            id: nextColumnId(
                columns.map(c => c.id),
                columnHint(path)
            ),
            label: pathLabel(info),
            path,
            aggregate: info.mode === 'aggregate' ? defaultAggregate(info.field, true) : '',
            scale: null,
            format: '',
        }
        builder.setColumns([...columns, column])
    }
    const rows = columns.map((column, index) => (
        <ColumnRow key={column.id} builder={builder} columns={columns} index={index} />
    ))
    return (
        <BuilderSection title="Columns" section="columns" problems={problems} isOpenAtStart>
            {rows}
            <View className="flex-row flex-wrap gap-2">
                <ColumnPicker
                    catalog={builder.catalog}
                    baseKey={builder.base.key}
                    onPick={onPick}
                    label="Add column"
                    testID="report-builder-add-column"
                />
            </View>
        </BuilderSection>
    )
}
