import { type Control, NumberInput, SelectInput, Toggle, useWatch } from '@tinycld/core/ui/form'
import { View } from 'react-native'
import type { ReportBuilder } from '../../hooks/useReportBuilder'
import { describePath, MAX_PIVOT_COLUMNS, pathLabel } from '../../lib/builder/columns'
import type { BuilderDraft } from '../../lib/builder/draft'
import { type PivotRangeChoice, pivotRangeChoices } from '../../lib/builder/pivot'
import type { SpecHop } from '../../lib/spec'
import type { Problem } from '../../lib/types'
import { BuilderSection } from './BuilderSection'
import { ColumnPicker } from './ColumnPicker'
import { BucketSelect, RecordLabelField } from './parts'
import { useColumnChoices } from './report-formula'

function usePivotFields(builder: ReportBuilder) {
    const { catalog, base, form } = builder
    const pivot = useWatch({ control: form.control, name: 'pivot' })
    const params = useWatch({ control: form.control, name: 'params' })
    const choices = useColumnChoices(builder)
    const isDatePath = (path: readonly SpecHop[]) => {
        const type = describePath(catalog, base.key, path)?.field.type
        return type === 'date' || type === 'datetime'
    }
    const info = describePath(catalog, base.key, pivot.path)
    const cells = choices
        .filter(choice => choice.hasAggregate)
        .map(choice => ({ value: choice.key, label: choice.label }))
    const rangeChoices = pivotRangeChoices({ source: base.key, params, pivot }, catalog)
    // A bucket left from a date field would reach the engine on a field that has
    // none, and a label field left from another record names a field it lacks.
    const onPick = (path: SpecHop[]) => {
        // A range belongs to a bucketed date; one left from it would be refused.
        if (!isDatePath(path)) {
            form.setValue('pivot.bucket', '', { shouldDirty: true })
            form.setValue('pivot.range', '', { shouldDirty: true })
        }
        const target = describePath(catalog, base.key, path)?.target
        if (target?.key !== info?.target?.key) {
            form.setValue('pivot.labelField', '', { shouldDirty: true })
        }
        builder.setPivotPath(path)
    }
    const setLabelField = (labelField: string) => {
        form.setValue('pivot.labelField', labelField, { shouldDirty: true })
        builder.validateLists()
    }
    const onBucketChange = (bucket: string) => {
        if (bucket === '') form.setValue('pivot.range', '', { shouldDirty: true })
    }
    return {
        isOn: pivot.on,
        fieldLabel: info ? pathLabel(info) : 'Choose a field',
        isDate: isDatePath(pivot.path),
        target: info?.target,
        labelField: pivot.labelField,
        cells,
        rangeChoices,
        onPick,
        setLabelField,
        onBucketChange,
    }
}

interface RangeSelectProps {
    choices: PivotRangeChoice[]
    control: Control<BuilderDraft>
}

// Every bucket of a date range, or only those with rows; shown where it applies, or
// where a range left from an earlier choice must be cleared.
function RangeSelect({ choices, control }: RangeSelectProps) {
    if (choices.length === 0) return null
    return (
        <SelectInput
            name="pivot.range"
            control={control}
            label="Columns"
            options={choices}
            testID="report-builder-pivot-range"
        />
    )
}

function PivotFields({ builder }: { builder: ReportBuilder }) {
    const { control } = builder.form
    const {
        isOn,
        fieldLabel,
        isDate,
        target,
        labelField,
        cells,
        rangeChoices,
        onPick,
        setLabelField,
        onBucketChange,
    } = usePivotFields(builder)
    if (!isOn) return null
    return (
        <View className="gap-2">
            <View className="flex-row">
                <ColumnPicker
                    catalog={builder.catalog}
                    baseKey={builder.base.key}
                    onPick={onPick}
                    label={fieldLabel}
                    testID="report-builder-pivot-field"
                    toOneOnly
                    allowLinkEnd
                />
            </View>
            <RecordLabelField
                target={target}
                labelField={labelField}
                purpose="Pivot columns shown as"
                testID="report-builder-pivot-label-field"
                onChange={setLabelField}
            />
            <BucketSelect
                isVisible={isDate}
                name="pivot.bucket"
                control={control}
                testID="report-builder-pivot-bucket"
                onValueChange={onBucketChange}
            />
            <RangeSelect choices={rangeChoices} control={control} />
            <SelectInput
                name="pivot.cell"
                control={control}
                label="Cells show"
                options={cells}
                testID="report-builder-pivot-cell"
                onValueChange={builder.validateLists}
            />
            <Toggle
                name="pivot.totalAfter"
                control={control}
                label="Total after the pivot columns"
                testID="report-builder-pivot-total-after"
                onValueChange={builder.validateLists}
            />
            <NumberInput
                name="pivot.maxColumns"
                control={control}
                label="At most this many columns"
                min={1}
                max={MAX_PIVOT_COLUMNS}
                testID="report-builder-pivot-max"
            />
        </View>
    )
}

// A pivot spreads a column with a total across one column per value of a field
// (a month, a status) or per linked record (a team). It needs a group level. It may show every bucket of a period, and its total after its columns.
export function PivotSection({
    builder,
    problems,
}: {
    builder: ReportBuilder
    problems: Problem[]
}) {
    return (
        <BuilderSection title="Pivot" section="pivot" problems={problems}>
            <Toggle
                name="pivot.on"
                control={builder.form.control}
                label="Pivot by a field"
                testID="report-builder-pivot-on"
                onValueChange={builder.validateLists}
            />
            <PivotFields builder={builder} />
        </BuilderSection>
    )
}
