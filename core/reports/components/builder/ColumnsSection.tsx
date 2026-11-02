import { Button, ButtonText } from '@tinycld/core/ui/button'
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
import type { ColumnDraft, FormulaDraft } from '../../lib/builder/draft'
import type { SpecHop } from '../../lib/spec'
import type { Aggregate, Problem } from '../../lib/types'
import { BuilderSection } from './BuilderSection'
import { ColumnPicker } from './ColumnPicker'
import { IconButton, NEW_FORMULA } from './parts'
import { ReportFormula, useColumnChoices } from './report-formula'

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

// A formula column is shown as a number or a percent when it gives a number; a
// formula giving text or a date has no such choice.
function isNumberColumn(
    builder: ReportBuilder,
    column: ColumnDraft,
    numericIds: ReadonlySet<string>
): boolean {
    if (column.kind === 'formula') return numericIds.has(column.id)
    return describePath(builder.catalog, builder.base.key, column.path)?.field.type === 'number'
}

interface ColumnRowProps {
    builder: ReportBuilder
    columns: ColumnDraft[]
    index: number
    /** The columns whose cells are numbers, amounts or percents. */
    numericIds: ReadonlySet<string>
}

function ColumnRow({ builder, columns, index, numericIds }: ColumnRowProps) {
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
            <FormatField builder={builder} column={column} index={index} numericIds={numericIds} />
            <ColumnFormula builder={builder} columns={columns} index={index} />
        </View>
    )
}

function FormatField({
    builder,
    column,
    index,
    numericIds,
}: {
    builder: ReportBuilder
    column: ColumnDraft
    index: number
    numericIds: ReadonlySet<string>
}) {
    if (!isNumberColumn(builder, column, numericIds)) return null
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

// The report formula editor (OQ2): every operator the engine has, its parts
// picked from menus, reading the other columns, fields, group records and params.
function ColumnFormula({ builder, columns, index }: Omit<ColumnRowProps, 'numericIds'>) {
    const column = columns[index]
    if (column?.kind !== 'formula') return null
    const setFormula = (formula: FormulaDraft) =>
        builder.setColumns(
            columns.map((c, i) => (i === index && c.kind === 'formula' ? { ...c, formula } : c))
        )
    return (
        <ReportFormula
            builder={builder}
            place={{ kind: 'column', selfKey: column.id }}
            formula={column.formula}
            onChange={setFormula}
            testID={`report-builder-column-${index}-formula`}
        />
    )
}

export function ColumnsSection({
    builder,
    problems,
}: {
    builder: ReportBuilder
    problems: Problem[]
}) {
    const columns = useWatch({ control: builder.form.control, name: 'columns' })
    const choices = useColumnChoices(builder)
    const numericIds = new Set(choices.filter(choice => choice.isNumeric).map(choice => choice.key))
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
            total: 'recompute',
            labelField: '',
        }
        builder.setColumns([...columns, column])
    }
    const onAddFormula = () =>
        builder.setColumns([
            ...columns,
            {
                kind: 'formula',
                id: nextColumnId(
                    columns.map(c => c.id),
                    'formula'
                ),
                label: 'Formula',
                formula: NEW_FORMULA,
                aggregate: '',
                scale: null,
                format: '',
                total: 'recompute',
                labelField: '',
            },
        ])
    const rows = columns.map((column, index) => (
        <ColumnRow
            key={column.id}
            builder={builder}
            columns={columns}
            index={index}
            numericIds={numericIds}
        />
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
                <Button
                    testID="report-builder-add-formula"
                    variant="outline"
                    size="sm"
                    onPress={onAddFormula}
                >
                    <ButtonText>Add formula</ButtonText>
                </Button>
            </View>
        </BuilderSection>
    )
}
