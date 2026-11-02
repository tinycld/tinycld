import { Button, ButtonText } from '@tinycld/core/ui/button'
import { SelectInput, TextInput, useWatch } from '@tinycld/core/ui/form'
import { type PickItem, PickMenu } from '@tinycld/core/ui/pick-menu'
import { Switch } from '@tinycld/core/ui/switch'
import { ArrowDown, ArrowUp, Trash2 } from 'lucide-react-native'
import { Text, View } from 'react-native'
import type { ReportBuilder } from '../../hooks/useReportBuilder'
import {
    AGGREGATE_LABELS,
    aggregatesFor,
    columnHint,
    defaultAggregate,
    describePath,
    firstTextField,
    linkTarget,
    nextColumnId,
    pathLabel,
} from '../../lib/builder/columns'
import { type ColumnDraft, type FormulaDraft, showsDetailLabel } from '../../lib/builder/draft'
import { rowsTotalColumns } from '../../lib/builder/formula-context'
import type { SpecHop } from '../../lib/spec'
import type { Aggregate, Problem } from '../../lib/types'
import { BuilderSection } from './BuilderSection'
import { ColumnPicker } from './ColumnPicker'
import { IconButton, NEW_FORMULA } from './parts'
import { ReportFormula, useColumnChoices } from './report-formula'
import { HopJoins } from './report-formula-paths'

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
    /** The columns whose total may add the rows shown (levels.go detailCell). */
    rowsTotalIds: ReadonlySet<string>
}

function ColumnRow({ builder, columns, index, numericIds, rowsTotalIds }: ColumnRowProps) {
    const source = useWatch({ control: builder.form.control, name: 'source' })
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
            <ColumnHops builder={builder} columns={columns} index={index} source={source} />
            <SelectInput
                name={`columns.${index}.aggregate`}
                control={control}
                label="Summarize"
                options={aggregateOptions(builder, column)}
                testID={`${testID}-aggregate`}
                onValueChange={builder.validateLists}
            />
            <FormatField builder={builder} column={column} index={index} numericIds={numericIds} />
            <TotalField
                isVisible={rowsTotalIds.has(column.id) || column.total === 'rows'}
                builder={builder}
                index={index}
            />
            <LinkLabelField builder={builder} columns={columns} index={index} source={source} />
            <ColumnFormula builder={builder} columns={columns} index={index} />
        </View>
    )
}

// A to-many hop turned into child rows has no total per row; one turned into a total
// starts at the field's usual total.
function withPath(
    builder: ReportBuilder,
    source: string,
    column: Extract<ColumnDraft, { kind: 'path' }>,
    path: SpecHop[]
): ColumnDraft {
    const before = describePath(builder.catalog, source, column.path)?.mode
    const info = describePath(builder.catalog, source, path)
    if (!info || info.mode === before) return { ...column, path }
    const aggregate = info.mode === 'aggregate' ? defaultAggregate(info.field, true) : ''
    const total = info.mode === 'nest' ? 'recompute' : column.total
    return { ...column, path, aggregate, scale: null, total }
}

interface ColumnPartProps {
    builder: ReportBuilder
    columns: ColumnDraft[]
    index: number
    source: string
}

// Each link of a path column: rows without it kept or dropped, and its one to-many
// link a total or child rows (no-limits ruling: every path is editable).
function ColumnHops({ builder, columns, index, source }: ColumnPartProps) {
    const column = columns[index]
    if (column?.kind !== 'path' || !column.path.some(hop => hop.link !== undefined)) return null
    const onChange = (path: SpecHop[]) =>
        builder.setColumns(
            columns.map((c, i) =>
                i === index && c.kind === 'path' ? withPath(builder, source, c, path) : c
            )
        )
    return (
        <View className="flex-row flex-wrap items-center gap-1.5">
            <HopJoins
                catalog={builder.catalog}
                baseKey={source}
                hops={column.path}
                onChange={onChange}
                id={`report-builder-column-${index}`}
                where={` in ${column.label}`}
            />
        </View>
    )
}

const TOTAL_OPTIONS = [
    { value: 'recompute', label: 'Recompute from all rows' },
    { value: 'rows', label: 'Add the rows shown' },
]

/**
 * A number or amount column totals all rows, or only the rows the report shows (E2).
 * A running value, a percent or child rows are not added up; a column that became
 * one after "Add the rows shown" keeps the choice in view, so it can be set back.
 */
function TotalField({
    isVisible,
    builder,
    index,
}: {
    isVisible: boolean
    builder: ReportBuilder
    index: number
}) {
    if (!isVisible) return null
    return (
        <SelectInput
            name={`columns.${index}.total`}
            control={builder.form.control}
            label="Total"
            options={TOTAL_OPTIONS}
            testID={`report-builder-column-${index}-total`}
            onValueChange={builder.validateLists}
        />
    )
}

/** "Show as": the field of a link column's record its cells show (R31, F22). */
function LinkLabelField({ builder, columns, index, source }: ColumnPartProps) {
    const column = columns[index]
    const target = column ? linkTarget(builder.catalog, source, column) : undefined
    if (!column || !target) return null
    const setField = (labelField: string) =>
        builder.setColumns(columns.map((c, i) => (i === index ? { ...c, labelField } : c)))
    const first = firstTextField(target)
    const items: PickItem[] = [
        {
            id: ':first',
            label: first ? `${first.label} (first text field)` : target.label,
            isSelected: column.labelField === '',
            onSelect: () => setField(''),
        },
        ...target.fields
            .filter(f => f.key !== 'id')
            .map(f => ({
                id: f.key,
                label: f.label,
                isSelected: f.key === column.labelField,
                onSelect: () => setField(f.key),
            })),
    ]
    const shown = items.find(i => i.isSelected)?.label ?? `${column.labelField} (missing)`
    return (
        <View className="flex-row items-center gap-2">
            <Text className="text-xs text-muted-foreground">Show as</Text>
            <PickMenu
                label={shown}
                purpose={`${column.label} shown as`}
                items={items}
                testID={`report-builder-column-${index}-label-field`}
            />
        </View>
    )
}

function DetailFormula({
    builder,
    formula,
    testID,
}: {
    builder: ReportBuilder
    formula: FormulaDraft | null
    testID: string
}) {
    if (!formula) return null
    return (
        <ReportFormula
            builder={builder}
            place={{ kind: 'detail' }}
            formula={formula}
            onChange={builder.setDetailLabel}
            testID={testID}
        />
    )
}

function DetailLabelError({ message }: { message: string | undefined }) {
    if (!message) return null
    return (
        <Text
            testID="report-builder-detail-label-error"
            role="alert"
            className="text-xs text-danger"
        >
            {message}
        </Text>
    )
}

/**
 * "Row label": opened rows are labelled by the first column, or by a formula (E7). A
 * pivot report has no opened rows (bind.go detail.label); a label set before the
 * pivot stays in view with its refusal, so it can be turned off.
 */
function DetailLabelCard({ builder }: { builder: ReportBuilder }) {
    const control = builder.form.control
    const label = useWatch({ control, name: 'detailLabel' })
    const pivot = useWatch({ control, name: 'pivot' })
    if (!showsDetailLabel({ pivot, detailLabel: label })) return null
    const error = builder.form.getFieldState('detailLabel', builder.form.formState).error
    const testID = 'report-builder-detail-label'
    return (
        <View testID={`${testID}-card`} className="gap-1.5 rounded-lg border border-border p-2">
            <DetailLabelError message={error?.message} />
            <View className="flex-row items-center gap-2">
                <Switch
                    value={label !== null}
                    onValueChange={on => builder.setDetailLabel(on ? NEW_FORMULA : null)}
                    accessibilityLabel="Label opened rows with a formula"
                    testID={`${testID}-on`}
                />
                <Text className="flex-1 text-sm text-foreground">
                    Label opened rows with a formula
                </Text>
            </View>
            <DetailFormula builder={builder} formula={label} testID={testID} />
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
            onValueChange={builder.validateLists}
        />
    )
}

// The report formula editor (OQ2): every operator the engine has, its parts
// picked from menus, reading the other columns, fields, group records and params.
function ColumnFormula({ builder, columns, index }: Omit<ColumnPartProps, 'source'>) {
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
    const rowsTotalIds = rowsTotalColumns(choices, columns)
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
            explicitTotal: false,
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
                explicitTotal: false,
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
            rowsTotalIds={rowsTotalIds}
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
                    allowLinkEnd
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
            <DetailLabelCard builder={builder} />
        </BuilderSection>
    )
}
