import { useWatch } from '@tinycld/core/ui/form'
import { type PickItem, PickMenu } from '@tinycld/core/ui/pick-menu'
import { Switch } from '@tinycld/core/ui/switch'
import { Text, View } from 'react-native'
import type { ReportBuilder } from '../../hooks/useReportBuilder'
import { firstTextField } from '../../lib/builder/columns'
import {
    type BuilderDraft,
    type FormulaDraft,
    type GroupLevelDraft,
    levelScope,
    showsHaving,
} from '../../lib/builder/draft'
import type { GroupDraft } from '../../lib/builder/filter'
import type { FormulaPlace } from '../../lib/builder/formula-context'
import type { ParamDef, SpecNode } from '../../lib/spec'
import type { CatalogSource } from '../../lib/types'
import { type AboveChoice, ConditionTree } from './ConditionTree'
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

// A label formula starts as Join text of what labels the row by default: the
// record's label field, or the group's value.
function startLabel(group: GroupLevelDraft, target: CatalogSource | undefined): FormulaDraft {
    const field = target ? (firstTextField(target)?.key ?? 'id') : undefined
    const value: SpecNode = field ? { record: group.id, path: [{ field }] } : { col: group.col }
    return { kind: 'editable', node: { op: 'concat', args: [value] } }
}

/** "Label": the default (a record's label field, or the value), another record field, or a formula (R9). */
export function GroupLabel({
    builder,
    groups,
    index,
    target,
    labelField,
}: LevelPartProps & { labelField: string }) {
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
                if (!isFormula) set(startLabel(group, target))
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
