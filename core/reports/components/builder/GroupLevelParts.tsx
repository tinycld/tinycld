import { Button, ButtonText } from '@tinycld/core/ui/button'
import { SelectInput, TextInput, useWatch } from '@tinycld/core/ui/form'
import { type PickItem, PickMenu } from '@tinycld/core/ui/pick-menu'
import { Switch } from '@tinycld/core/ui/switch'
import type { ReactNode } from 'react'
import { Text, View } from 'react-native'
import type { ReportBuilder } from '../../hooks/useReportBuilder'
import { firstTextField } from '../../lib/builder/columns'
import {
    type BuilderDraft,
    type FormulaDraft,
    type GroupLevelDraft,
    labelStart,
    levelScope,
    type OpeningDraft,
    openingCellRows,
    openingDraft,
    setOpeningCell,
    showsBlankSkip,
    showsHaving,
    showsHideEmpty,
} from '../../lib/builder/draft'
import type { GroupDraft } from '../../lib/builder/filter'
import type { FormulaPlace } from '../../lib/builder/formula-context'
import type { ParamDef, SpecNode } from '../../lib/spec'
import type { CatalogSource } from '../../lib/types'
import { type AboveChoice, ConditionTree } from './ConditionTree'
import { FieldError, NEW_FORMULA } from './parts'
import { ReportFormula } from './report-formula'

// The parts of a group level after its column: its label, "Show every <record>"
// with its conditions (a fill, R6), and "Show only groups where…" (having, R18).

/** The group's own field of its record, when the formula is just that ({record, path: [field]}). */
export function bareRecordField(node: SpecNode | undefined, group: string): string | undefined {
    const [hop] = node?.path ?? []
    const isBare =
        node?.record === group && node.path?.length === 1 && Object.keys(node).length === 2
    return isBare ? hop?.field : undefined
}

export interface LevelPartProps {
    builder: ReportBuilder
    groups: BuilderDraft['groups']
    index: number
    target: CatalogSource | undefined
}

export const setLevel = (
    builder: ReportBuilder,
    groups: BuilderDraft['groups'],
    index: number,
    patch: Partial<GroupLevelDraft>,
    ...added: ParamDef[]
) =>
    builder.setGroups(
        groups.map((g, i) => (i === index ? { ...g, ...patch } : g)),
        ...added
    )

/** A level formula: a group's label (its level's place) or a sort (every level). */
export function LevelFormula({
    isVisible,
    builder,
    place,
    formula,
    onChange,
    testID,
}: {
    isVisible: boolean
    builder: ReportBuilder
    place: Extract<FormulaPlace, { kind: 'level' }>
    formula: FormulaDraft | null
    onChange: (formula: FormulaDraft) => void
    testID: string
}) {
    if (!isVisible || !formula) return null
    return (
        <ReportFormula
            builder={builder}
            place={place}
            formula={formula}
            onChange={onChange}
            testID={testID}
        />
    )
}

/** "Label": the default (a record's label field, or the value), another record field, or a formula (R9). */
export function GroupLabel({
    builder,
    groups,
    index,
    target,
    labelField,
}: LevelPartProps & { labelField: string }) {
    const columns = useWatch({ control: builder.form.control, name: 'columns' })
    const group = groups[index]
    if (!group) return null
    const testID = `report-builder-group-${index}-label`
    const field = bareRecordField(group.label?.node, group.id)
    const isFormula = group.label !== null && field === undefined
    const set = (label: FormulaDraft | null) => setLevel(builder, groups, index, { label })
    const shownField =
        target?.fields.find(f => f.key === labelField) ?? (target && firstTextField(target))
    const fields = (target?.fields ?? []).filter(f => f.key !== 'id')
    const items: PickItem[] = [
        {
            id: ':default',
            label: target ? `Default (${shownField?.label ?? 'ID'})` : 'Default (the value)',
            isSelected: group.label === null,
            onSelect: () => set(null),
        },
        ...fields.map(f => ({
            id: f.key,
            label: f.label,
            isSelected: f.key === field,
            onSelect: () =>
                set({ kind: 'editable', node: { record: group.id, path: [{ field: f.key }] } }),
        })),
        {
            id: ':formula',
            label: 'Formula…',
            isSelected: isFormula,
            onSelect: () => {
                if (!isFormula) set(labelStart(group, target, columns))
            },
        },
    ]
    const shown = items.find(i => i.isSelected)?.label ?? `${field} (missing)`
    return (
        <View className="gap-1">
            <View className="flex-row items-center gap-2">
                <Text className="text-xs text-muted-foreground">Label</Text>
                <PickMenu label={shown} purpose="Group label" items={items} testID={testID} />
            </View>
            <LevelFormula
                isVisible={isFormula}
                builder={builder}
                place={{ kind: 'level', index }}
                formula={group.label}
                onChange={set}
                testID={`${testID}-formula`}
            />
        </View>
    )
}

function FillConditions({
    isVisible,
    builder,
    root,
    baseKey,
    testID,
    above,
    error,
    onChange,
}: {
    isVisible: boolean
    builder: ReportBuilder
    root: GroupDraft
    baseKey: string
    testID: string
    above: readonly AboveChoice[]
    error: string | undefined
    onChange: (where: GroupDraft, ...added: ParamDef[]) => void
}) {
    if (!isVisible) return null
    return (
        <ConditionTree
            builder={builder}
            root={root}
            onChange={onChange}
            baseKey={baseKey}
            testID={testID}
            above={above}
            error={error}
        />
    )
}

/** "Show every <target>, even with no rows", with its own conditions, on any level (R6). */
export function GroupFill({
    builder,
    groups,
    index,
    target,
    above,
}: LevelPartProps & { above: readonly AboveChoice[] }) {
    const group = groups[index]
    if (!group || !target) return null
    const testID = `report-builder-group-${index}-fill`
    const state = builder.form.getFieldState(`groups.${index}.fill`, builder.form.formState)
    const setFill = (fill: GroupLevelDraft['fill'], ...added: ParamDef[]) =>
        setLevel(builder, groups, index, { fill }, ...added)
    return (
        <View className="gap-1.5">
            <View className="flex-row items-center gap-2">
                <Switch
                    value={group.fill.on}
                    onValueChange={on => setFill({ ...group.fill, on })}
                    accessibilityLabel={`Show every ${target.label}`}
                    testID={`${testID}-on`}
                />
                <Text className="flex-1 text-sm text-foreground">
                    {`Show every ${target.label}, even with no rows`}
                </Text>
            </View>
            <FillConditions
                isVisible={group.fill.on}
                builder={builder}
                root={group.fill.where}
                baseKey={target.key}
                testID={testID}
                above={above}
                error={state.error?.message}
                onChange={(where, ...added) => setFill({ on: true, where }, ...added)}
            />
        </View>
    )
}

/**
 * "Show only groups where…": conditions on the level's totals, the columns with a
 * value on its rows, and the records of its group and the groups around it (R18).
 * A layout's first group has none (its sections are fixed rows); one read back
 * there stays in view, with its refusal, so it can be removed.
 */
export function GroupHaving({ builder, groups, index }: Omit<LevelPartProps, 'target'>) {
    const control = builder.form.control
    const source = useWatch({ control, name: 'source' })
    const columns = useWatch({ control, name: 'columns' })
    const params = useWatch({ control, name: 'params' })
    const layout = useWatch({ control, name: 'layout' })
    const group = groups[index]
    if (!group || !showsHaving({ layout, groups }, index)) return null
    // Read with useWatch, so the condition menus follow other sections (F27).
    const level = levelScope({ source, columns, groups, params }, builder.catalog, index)
    const state = builder.form.getFieldState(`groups.${index}.having`, builder.form.formState)
    return (
        <View className="gap-1">
            <Text className="text-xs text-muted-foreground">Show only groups where…</Text>
            <ConditionTree
                builder={builder}
                root={group.having}
                baseKey={source}
                testID={`report-builder-group-${index}-having`}
                level={level}
                group={group.id}
                error={state.error?.message}
                onChange={(having, ...added) =>
                    setLevel(builder, groups, index, { having }, ...added)
                }
            />
        </View>
    )
}

const BLANK_OPTIONS = [
    { value: 'group', label: "Put them in a '(none)' group" },
    { value: 'skip', label: 'Place them directly under the group above' },
]

/**
 * "Rows with no <col>": a '(none)' group, or directly under the group above (G1).
 * A skip read back where bind refuses it stays in view, with its refusal, so it can
 * be set back.
 */
export function GroupBlank({ builder, groups, index }: Omit<LevelPartProps, 'target'>) {
    const columns = useWatch({ control: builder.form.control, name: 'columns' })
    const group = groups[index]
    if (!group || !showsBlankSkip({ groups }, index)) return null
    const columnLabel = columns.find(c => c.id === group.col)?.label ?? group.col
    return (
        <SelectInput
            name={`groups.${index}.blank`}
            control={builder.form.control}
            label={`Rows with no ${columnLabel}`}
            options={BLANK_OPTIONS}
            // An absent blank is the engine's default, a '(none)' group.
            placeholder={BLANK_OPTIONS[0]?.label}
            testID={`report-builder-group-${index}-blank`}
            onValueChange={builder.validateLists}
        />
    )
}

const HIDE_EMPTY_LABEL = 'Hide a group when every row in it is hidden'

/**
 * "Hide a group when every row in it is hidden" (G5). Turned on where bind refuses
 * it, it stays in view with its refusal, so it can be turned off.
 */
export function GroupHideEmpty({ builder, groups, index }: Omit<LevelPartProps, 'target'>) {
    const layout = useWatch({ control: builder.form.control, name: 'layout' })
    const group = groups[index]
    if (!group || !showsHideEmpty({ groups, layout }, index)) return null
    const testID = `report-builder-group-${index}-hide-empty`
    const state = builder.form.getFieldState(`groups.${index}.hideEmpty`, builder.form.formState)
    return (
        <View className="gap-1">
            <View className="flex-row items-center gap-2">
                <Switch
                    value={group.hideEmpty === true}
                    onValueChange={hideEmpty => setLevel(builder, groups, index, { hideEmpty })}
                    accessibilityLabel={HIDE_EMPTY_LABEL}
                    testID={testID}
                />
                <Text className="flex-1 text-sm text-foreground">{HIDE_EMPTY_LABEL}</Text>
            </View>
            <FieldError message={state.error?.message} testID={`${testID}-error`} />
        </View>
    )
}

interface TextActionProps {
    isVisible: boolean
    text: string
    label: string
    onPress: () => void
    testID: string
}

function TextAction({ isVisible, text, label, onPress, testID }: TextActionProps) {
    if (!isVisible) return null
    return (
        <Button
            variant="outline"
            size="sm"
            onPress={onPress}
            accessibilityLabel={label}
            testID={testID}
        >
            <ButtonText>{text}</ButtonText>
        </Button>
    )
}

type OpeningCellRow = ReturnType<typeof openingCellRows>[number]

interface OpeningCellProps {
    builder: ReportBuilder
    index: number
    row: OpeningCellRow
    opening: OpeningDraft
    onChange: (opening: OpeningDraft) => void
    testID: string
}

function openingCellCaption(row: OpeningCellRow, hasValue: boolean) {
    if (!hasValue) return `${row.label}: No value`
    return `${row.label}:`
}

// One column of the opening row: no value, or a formula worked out on the level's
// rows (levels.go levelCtx: totals, group fields, lookups, asked values). A stale
// cell (Save refuses it) shows its refusal and Remove only.
function OpeningCell({ builder, index, row, opening, onChange, testID }: OpeningCellProps) {
    const formula = opening.cells[row.col] ?? null
    const set = (next: FormulaDraft | null) => onChange(setOpeningCell(opening, row.col, next))
    const state = builder.form.getFieldState(
        `groups.${index}.opening.cells.${row.col}`,
        builder.form.formState
    )
    return (
        <View testID={testID} className="gap-1">
            <View className="flex-row items-center gap-2">
                <Text numberOfLines={1} className="min-w-0 flex-1 text-sm text-foreground">
                    {openingCellCaption(row, formula !== null)}
                </Text>
                <TextAction
                    isVisible={formula === null}
                    text="Add a value"
                    label={`Add a value for ${row.label}`}
                    onPress={() => set(NEW_FORMULA)}
                    testID={`${testID}-add`}
                />
                <TextAction
                    isVisible={formula !== null}
                    text="Remove"
                    label={`Remove the value for ${row.label}`}
                    onPress={() => set(null)}
                    testID={`${testID}-remove`}
                />
            </View>
            <FieldError message={state.error?.message} testID={`${testID}-error`} />
            <LevelFormula
                isVisible={!row.isStale}
                builder={builder}
                place={{ kind: 'level', index }}
                formula={formula}
                onChange={set}
                testID={`${testID}-formula`}
            />
        </View>
    )
}

interface OpeningFieldsProps {
    isVisible: boolean
    builder: ReportBuilder
    index: number
    testID: string
    cells: ReactNode
}

function OpeningFields({ isVisible, builder, index, testID, cells }: OpeningFieldsProps) {
    if (!isVisible) return null
    return (
        <View className="gap-1.5">
            <TextInput
                name={`groups.${index}.opening.label`}
                control={builder.form.control}
                label="Row label"
                testID={`${testID}-label`}
                maxLength={200}
            />
            {cells}
        </View>
    )
}

/**
 * "Start each group with a row" (F5): a row first under each opened group of the
 * level, with its own label and a value in any own-row column after the first (the
 * label fills the first). It shows also under a group with no rows of its own.
 */
export function GroupOpening({ builder, groups, index }: Omit<LevelPartProps, 'target'>) {
    const columns = useWatch({ control: builder.form.control, name: 'columns' })
    const group = groups[index]
    if (!group) return null
    const testID = `report-builder-group-${index}-opening`
    const opening = group.opening ?? null
    const state = builder.form.getFieldState(`groups.${index}.opening`, builder.form.formState)
    const setOpening = (next: OpeningDraft | null) =>
        setLevel(builder, groups, index, { opening: next })
    const cells = opening
        ? openingCellRows(columns, opening).map(row => (
              <OpeningCell
                  key={row.col}
                  builder={builder}
                  index={index}
                  row={row}
                  opening={opening}
                  onChange={setOpening}
                  testID={`${testID}-cell-${row.col}`}
              />
          ))
        : null
    return (
        <View className="gap-1.5">
            <View className="flex-row items-center gap-2">
                <Switch
                    value={opening !== null}
                    onValueChange={on => setOpening(on ? openingDraft('Opening') : null)}
                    accessibilityLabel="Start each group with a row"
                    testID={testID}
                />
                <Text className="flex-1 text-sm text-foreground">Start each group with a row</Text>
            </View>
            <FieldError message={state.error?.message} testID={`${testID}-error`} />
            <OpeningFields
                isVisible={opening !== null}
                builder={builder}
                index={index}
                testID={testID}
                cells={cells}
            />
        </View>
    )
}
