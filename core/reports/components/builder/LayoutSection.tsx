import { Button, ButtonText } from '@tinycld/core/ui/button'
import { SelectInput, TextInput, useWatch } from '@tinycld/core/ui/form'
import { ArrowDown, ArrowUp, Trash2 } from 'lucide-react-native'
import { Text, View } from 'react-native'
import type { ReportBuilder } from '../../hooks/useReportBuilder'
import { MAX_LAYOUT_ITEMS, nextColumnId } from '../../lib/builder/columns'
import {
    cellColumnIds,
    type FormulaDraft,
    type LayoutItemDraft,
    withinChoices,
} from '../../lib/builder/draft'
import { columnTypes } from '../../lib/builder/formula-context'
import type { Problem } from '../../lib/types'
import { BuilderSection } from './BuilderSection'
import { moved } from './ColumnsSection'
import { FieldError, IconButton, NEW_FORMULA } from './parts'
import { ReportFormula, useColumnChoices } from './report-formula'

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

// A row with column formulas may drop its formula for the other columns; a row
// with none keeps it, as it needs one or the other.
function RemoveRowFormula({
    isVisible,
    onPress,
    testID,
}: {
    isVisible: boolean
    onPress: () => void
    testID: string
}) {
    if (!isVisible) return null
    return (
        <View className="flex-row items-center gap-2">
            <Text numberOfLines={1} className="min-w-0 flex-1 text-xs text-muted-foreground">
                The other columns
            </Text>
            <Button
                variant="outline"
                size="sm"
                onPress={onPress}
                accessibilityLabel="Remove the formula for the other columns"
                testID={testID}
            >
                <ButtonText>Remove</ButtonText>
            </Button>
        </View>
    )
}

// A computed row reads the sections and computed rows above it, by id, and may
// look values up (F16).
function RowFormula({ builder, items, index }: ItemProps) {
    const item = items[index]
    if (item?.kind !== 'row') return null
    const setFormula = (formula: FormulaDraft | null) =>
        builder.setLayout(
            items.map((other, i) =>
                i === index && other.kind === 'row' ? { ...other, formula } : other
            )
        )
    const addFormula = () => setFormula(NEW_FORMULA)
    const removeFormula = () => setFormula(null)
    // Save's refusals of the row's formula (none and no column formula, or a
    // read of its own section) are drawn here, in both forms of the row.
    const state = builder.form.getFieldState(`layout.${index}.formula`, builder.form.formState)
    const error = (
        <FieldError
            message={state.error?.message}
            testID={`report-builder-layout-${index}-formula-error`}
        />
    )
    // A row with column formulas may have none for the other columns.
    if (item.formula === null) {
        return (
            <View className="gap-1">
                {error}
                <Button
                    variant="outline"
                    size="sm"
                    onPress={addFormula}
                    testID={`report-builder-layout-${index}-formula-add`}
                >
                    <ButtonText>Add a formula for the other columns</ButtonText>
                </Button>
            </View>
        )
    }
    const hasCells = Object.keys(item.cells ?? {}).length > 0
    return (
        <View className="gap-1">
            <RemoveRowFormula
                isVisible={hasCells}
                onPress={removeFormula}
                testID={`report-builder-layout-${index}-formula-remove`}
            />
            {error}
            <ReportFormula
                builder={builder}
                place={{ kind: 'layout', index }}
                formula={item.formula}
                onChange={setFormula}
                testID={`report-builder-layout-${index}-formula`}
            />
        </View>
    )
}

const AFTER_SECTIONS = 'After the sections'

/**
 * "Add a row to this section": a section above the row, or after the sections (G4).
 * A section read back that is no longer above the row stays in view, with its
 * refusal, so it can be set back.
 */
function RowWithin({ builder, items, index }: ItemProps) {
    const item = items[index]
    if (item?.kind !== 'row') return null
    const sections = withinChoices({ layout: items }, index)
    const within = item.within ?? ''
    if (sections.length === 0 && within === '') return null
    const titleOf = (id: string) => {
        const found = items.find(other => other.kind === 'section' && other.id === id)
        return found?.kind === 'section' ? found.title : id
    }
    const options = [
        { value: '', label: AFTER_SECTIONS },
        ...sections.map(id => ({ value: id, label: titleOf(id) })),
    ]
    return (
        <SelectInput
            name={`layout.${index}.within`}
            control={builder.form.control}
            label="Add a row to this section"
            options={options}
            // An absent within places the row after the sections.
            placeholder={AFTER_SECTIONS}
            testID={`report-builder-layout-${index}-within`}
            onValueChange={builder.validateLists}
        />
    )
}

interface CellRowData {
    col: string
    label: string
    formula: FormulaDraft | null
    /** A cell in a column "This column: …" no longer offers: Save refuses it. */
    isStale: boolean
}

function CellFormula({
    isVisible,
    builder,
    index,
    formula,
    onChange,
    testID,
}: {
    isVisible: boolean
    builder: ReportBuilder
    index: number
    formula: FormulaDraft | null
    onChange: (formula: FormulaDraft) => void
    testID: string
}) {
    if (!isVisible || !formula) return null
    return (
        <ReportFormula
            builder={builder}
            place={{ kind: 'layout', index }}
            formula={formula}
            onChange={onChange}
            testID={testID}
        />
    )
}

// One column of a computed row: the row's formula, or its own formula for this
// column. A stale cell shows its refusal and Remove only.
function CellRow({
    builder,
    index,
    row,
    onChange,
}: {
    builder: ReportBuilder
    index: number
    row: CellRowData
    onChange: (formula: FormulaDraft | null) => void
}) {
    const testID = `report-builder-layout-${index}-cell-${row.col}`
    const caption = `This column: ${row.label}`
    if (!row.formula) {
        return (
            <Button
                variant="outline"
                size="sm"
                onPress={() => onChange(NEW_FORMULA)}
                testID={`${testID}-add`}
            >
                <ButtonText>{caption}</ButtonText>
            </Button>
        )
    }
    const state = builder.form.getFieldState(
        `layout.${index}.cells.${row.col}`,
        builder.form.formState
    )
    return (
        <View className="gap-1">
            <View className="flex-row items-center gap-2">
                <Text numberOfLines={1} className="min-w-0 flex-1 text-xs text-muted-foreground">
                    {caption}
                </Text>
                <Button
                    variant="outline"
                    size="sm"
                    onPress={() => onChange(null)}
                    accessibilityLabel={`Remove the formula for ${row.label}`}
                    testID={`${testID}-remove`}
                >
                    <ButtonText>Remove</ButtonText>
                </Button>
            </View>
            <FieldError message={state.error?.message} testID={`${testID}-error`} />
            <CellFormula
                isVisible={!row.isStale}
                builder={builder}
                index={index}
                formula={row.formula}
                onChange={onChange}
                testID={testID}
            />
        </View>
    )
}

/** "This column: …": the row's own formula in a number, amount or percent column (G8). */
function RowCells({ builder, items, index }: ItemProps) {
    const control = builder.form.control
    const source = useWatch({ control, name: 'source' })
    const columns = useWatch({ control, name: 'columns' })
    const params = useWatch({ control, name: 'params' })
    const groups = useWatch({ control, name: 'groups' })
    const pivot = useWatch({ control, name: 'pivot' })
    const item = items[index]
    if (item?.kind !== 'row') return null
    const types = columnTypes({ source, columns, params, groups }, builder.catalog)
    const offered = cellColumnIds({ columns, pivot }, types)
    const cells = item.cells ?? {}
    const stale = Object.keys(cells).filter(col => !offered.includes(col))
    const setCell = (col: string, formula: FormulaDraft | null) => {
        const kept = Object.entries(cells).filter(([key]) => key !== col)
        const next = Object.fromEntries(formula ? [...kept, [col, formula] as const] : kept)
        builder.setLayout(
            items.map((other, i) =>
                i === index && other.kind === 'row' ? { ...other, cells: next } : other
            )
        )
    }
    const rows = [...offered, ...stale].map(col => (
        <CellRow
            key={col}
            builder={builder}
            index={index}
            row={{
                col,
                label: columns.find(c => c.id === col)?.label ?? col,
                formula: cells[col] ?? null,
                isStale: stale.includes(col),
            }}
            onChange={formula => setCell(col, formula)}
        />
    ))
    return <View className="gap-1">{rows}</View>
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
            <RowWithin {...props} />
            <RowCells {...props} />
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
    const firstGroup = useColumnChoices(builder).find(choice => choice.key === groups[0]?.col)
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
