import { FormulaEditor } from '@tinycld/core/components/forms-builder/FormulaEditor'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { SelectInput, TextInput, useWatch } from '@tinycld/core/ui/form'
import { ArrowDown, ArrowUp, Trash2 } from 'lucide-react-native'
import { Text, View } from 'react-native'
import type { ReportBuilder } from '../../hooks/useReportBuilder'
import { MAX_LAYOUT_ITEMS, nextColumnId } from '../../lib/builder/columns'
import { columnChoices, type FormulaDraft, type LayoutItemDraft } from '../../lib/builder/draft'
import { formulaOperands } from '../../lib/builder/formula'
import type { Problem } from '../../lib/types'
import { BuilderSection } from './BuilderSection'
import { moved } from './ColumnsSection'
import { FixedFormula, IconButton, NEW_FORMULA } from './parts'

const KIND_LABELS = { section: 'Section', row: 'Computed row' } as const

function NeedsGroup({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return (
        <Text className="text-sm text-muted-foreground">
            A layout shows the first group level's values as sections. Add a group level first.
        </Text>
    )
}

interface ItemProps {
    builder: ReportBuilder
    items: LayoutItemDraft[]
    index: number
    keyOptions: { value: string; label: string }[]
}

// A section's key is a value of the first group level: one of a select's options
// when the group column is a select, else typed.
function SectionKey({ builder, index, keyOptions }: ItemProps) {
    const testID = `report-builder-layout-${index}-key`
    const { control } = builder.form
    if (keyOptions.length > 0) {
        return (
            <SelectInput
                name={`layout.${index}.key`}
                control={control}
                label="Shows the group"
                options={keyOptions}
                testID={testID}
            />
        )
    }
    return (
        <TextInput
            name={`layout.${index}.key`}
            control={control}
            label="Shows the group"
            testID={testID}
        />
    )
}

// A computed row reads the sections and computed rows above it, by id.
function RowFormula({ builder, items, index }: ItemProps) {
    const item = items[index]
    if (item?.kind !== 'row') return null
    const testID = `report-builder-layout-${index}-formula`
    const setFormula = (formula: FormulaDraft) =>
        builder.setLayout(
            items.map((other, i) =>
                i === index && other.kind === 'row' ? { ...other, formula } : other
            )
        )
    if (item.formula.kind === 'fixed') {
        return <FixedFormula testID={`${testID}-fixed`} onRemove={() => setFormula(NEW_FORMULA)} />
    }
    const earlier = items.slice(0, index).map(other => ({
        key: other.id,
        label: other.kind === 'section' ? other.title : other.label,
        isNumeric: true,
    }))
    return (
        <FormulaEditor
            value={item.formula.expr}
            onChange={expr => setFormula({ kind: 'editable', expr })}
            operands={formulaOperands(earlier, item.id)}
            valueNoun="Section or row"
            testID={testID}
        />
    )
}

function ItemFields(props: ItemProps) {
    const { builder, items, index } = props
    const item = items[index]
    const { control } = builder.form
    const testID = `report-builder-layout-${index}`
    if (item?.kind === 'section') {
        return (
            <>
                <TextInput
                    name={`layout.${index}.title`}
                    control={control}
                    label="Section title"
                    testID={`${testID}-title`}
                    maxLength={200}
                />
                <SectionKey {...props} />
            </>
        )
    }
    return (
        <>
            <TextInput
                name={`layout.${index}.label`}
                control={control}
                label="Row label"
                testID={`${testID}-label`}
                maxLength={200}
            />
            <RowFormula {...props} />
        </>
    )
}

function LayoutItem(props: ItemProps) {
    const { builder, items, index } = props
    const item = items[index]
    if (!item) return null
    const testID = `report-builder-layout-${index}`
    return (
        <View testID={testID} className="gap-1 rounded-lg border border-border p-2">
            <View className="flex-row items-center">
                <Text className="flex-1 text-xs text-muted-foreground">
                    {KIND_LABELS[item.kind]}
                </Text>
                <IconButton
                    label="Move up"
                    testID={`${testID}-up`}
                    isVisible={index > 0}
                    icon={ArrowUp}
                    onPress={() => builder.setLayout(moved(items, index, index - 1))}
                />
                <IconButton
                    label="Move down"
                    testID={`${testID}-down`}
                    isVisible={index < items.length - 1}
                    icon={ArrowDown}
                    onPress={() => builder.setLayout(moved(items, index, index + 1))}
                />
                <IconButton
                    label="Remove"
                    testID={`${testID}-remove`}
                    isVisible
                    icon={Trash2}
                    onPress={() => builder.setLayout(items.filter((_, i) => i !== index))}
                />
            </View>
            <ItemFields {...props} />
        </View>
    )
}

function useLayout(builder: ReportBuilder) {
    const { control } = builder.form
    const items = useWatch({ control, name: 'layout' })
    const groups = useWatch({ control, name: 'groups' })
    const columns = useWatch({ control, name: 'columns' })
    const firstGroup = columnChoices({ source: builder.base.key, columns }, builder.catalog).find(
        choice => choice.key === groups[0]?.col
    )
    const ids = items.map(item => item.id)
    return {
        items,
        keyOptions: firstGroup?.options ?? [],
        hasGroup: groups.length > 0,
        canAdd: items.length < MAX_LAYOUT_ITEMS,
        addSection: () =>
            builder.setLayout([
                ...items,
                { kind: 'section', id: nextColumnId(ids, 'section'), title: 'Section', key: '' },
            ]),
        addRow: () =>
            builder.setLayout([
                ...items,
                {
                    kind: 'row',
                    id: nextColumnId(ids, 'row'),
                    label: 'Computed row',
                    formula: NEW_FORMULA,
                },
            ]),
    }
}

function AddButtons({
    isVisible,
    onAddSection,
    onAddRow,
}: {
    isVisible: boolean
    onAddSection: () => void
    onAddRow: () => void
}) {
    if (!isVisible) return null
    return (
        <View className="flex-row flex-wrap gap-2">
            <Button
                testID="report-builder-layout-add-section"
                variant="outline"
                size="sm"
                onPress={onAddSection}
            >
                <ButtonText>Add section</ButtonText>
            </Button>
            <Button
                testID="report-builder-layout-add-row"
                variant="outline"
                size="sm"
                onPress={onAddRow}
            >
                <ButtonText>Add computed row</ButtonText>
            </Button>
        </View>
    )
}

// Statements: an ordered list of sections, each the rows of one first-level group
// with its subtotal, and computed rows over sections by id (spec §2 "layout").
// A layout report ends in its computed rows; it has no Total row.
export function LayoutSection({
    builder,
    problems,
}: {
    builder: ReportBuilder
    problems: Problem[]
}) {
    const layout = useLayout(builder)
    const rows = layout.items.map((item, index) => (
        <LayoutItem
            key={item.id}
            builder={builder}
            items={layout.items}
            index={index}
            keyOptions={layout.keyOptions}
        />
    ))
    return (
        <BuilderSection title="Layout" section="layout" problems={problems}>
            <NeedsGroup isVisible={!layout.hasGroup} />
            {rows}
            <AddButtons
                isVisible={layout.canAdd}
                onAddSection={layout.addSection}
                onAddRow={layout.addRow}
            />
        </BuilderSection>
    )
}
