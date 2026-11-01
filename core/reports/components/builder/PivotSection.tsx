import { NumberInput, SelectInput, Toggle, useWatch } from '@tinycld/core/ui/form'
import { View } from 'react-native'
import type { ReportBuilder } from '../../hooks/useReportBuilder'
import { describePath, MAX_PIVOT_COLUMNS, pathLabel } from '../../lib/builder/columns'
import { columnChoices } from '../../lib/builder/draft'
import type { SpecHop } from '../../lib/spec'
import type { Problem } from '../../lib/types'
import { BuilderSection } from './BuilderSection'
import { ColumnPicker } from './ColumnPicker'
import { BucketSelect } from './parts'

function usePivotFields(builder: ReportBuilder) {
    const { catalog, base, form } = builder
    const pivot = useWatch({ control: form.control, name: 'pivot' })
    const columns = useWatch({ control: form.control, name: 'columns' })
    const isDatePath = (path: readonly SpecHop[]) => {
        const type = describePath(catalog, base.key, path)?.field.type
        return type === 'date' || type === 'datetime'
    }
    const info = describePath(catalog, base.key, pivot.path)
    const cells = columnChoices({ source: base.key, columns }, catalog)
        .filter(choice => choice.hasAggregate)
        .map(choice => ({ value: choice.key, label: choice.label }))
    // A bucket left from a date field would reach the engine on a field that has none.
    const onPick = (path: SpecHop[]) => {
        if (!isDatePath(path)) form.setValue('pivot.bucket', '', { shouldDirty: true })
        builder.setPivotPath(path)
    }
    return {
        isOn: pivot.on,
        fieldLabel: info ? pathLabel(info) : 'Choose a field',
        isDate: isDatePath(pivot.path),
        cells,
        onPick,
    }
}

function PivotFields({ builder }: { builder: ReportBuilder }) {
    const { control } = builder.form
    const { isOn, fieldLabel, isDate, cells, onPick } = usePivotFields(builder)
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
                />
            </View>
            <BucketSelect
                isVisible={isDate}
                name="pivot.bucket"
                control={control}
                testID="report-builder-pivot-bucket"
            />
            <SelectInput
                name="pivot.cell"
                control={control}
                label="Cells show"
                options={cells}
                testID="report-builder-pivot-cell"
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
// (a month, a status). It needs a group level.
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
            />
            <PivotFields builder={builder} />
        </BuilderSection>
    )
}
