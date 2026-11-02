import { Button, ButtonText } from '@tinycld/core/ui/button'
import { ButtonGroup, SelectInput, Toggle, useWatch } from '@tinycld/core/ui/form'
import { type PickItem, PickMenu } from '@tinycld/core/ui/pick-menu'
import { Switch } from '@tinycld/core/ui/switch'
import { ArrowDown, ArrowUp, Trash2 } from 'lucide-react-native'
import { Text, View } from 'react-native'
import type { ReportBuilder } from '../../hooks/useReportBuilder'
import {
    type Catalog,
    firstTextField,
    MAX_GROUP_LEVELS,
    MAX_SORT_KEYS,
    nextColumnId,
} from '../../lib/builder/columns'
import {
    type BuilderDraft,
    type FormulaDraft,
    type GroupLevelDraft,
    groupTarget,
    levelScope,
    type SortDraft,
} from '../../lib/builder/draft'
import { aboveOf, emptyGroup, type GroupDraft } from '../../lib/builder/filter'
import { type ColumnChoice as Choice, recordGroups } from '../../lib/builder/formula-context'
import type { FormulaRecord } from '../../lib/builder/formula-parts'
import type { ParamDef, SpecNode } from '../../lib/spec'
import type { CatalogSource, Problem } from '../../lib/types'
import { BuilderSection } from './BuilderSection'
import { moved } from './ColumnsSection'
import { type AboveChoice, ConditionTree } from './ConditionTree'
import { BucketSelect, IconButton, NEW_FORMULA } from './parts'
import { ReportFormula, useColumnChoices } from './report-formula'

const DIR_OPTIONS = [
    { value: 'asc', label: 'A to Z' },
    { value: 'desc', label: 'Z to A' },
]

const asOption = (choice: Choice) => ({ value: choice.key, label: choice.label })

function AddButton({
    isVisible,
    label,
    testID,
    onPress,
}: {
    isVisible: boolean
    label: string
    testID: string
    onPress: () => void
}) {
    if (!isVisible) return null
    return (
        <Button
            testID={testID}
            variant="outline"
            size="sm"
            onPress={onPress}
            className="self-start"
        >
            <ButtonText>{label}</ButtonText>
        </Button>
    )
}

function FieldError({ message, testID }: { message: string | undefined; testID: string }) {
    if (!message) return null
    return (
        <Text testID={testID} role="alert" className="text-xs text-danger">
            {message}
        </Text>
    )
}

/** The group's own field of its record, when the formula is just that ({record, path: [field]}). */
function bareRecordField(node: SpecNode | undefined, group: string): string | undefined {
    const [hop] = node?.path ?? []
    const isBare =
        node?.record === group && node.path?.length === 1 && Object.keys(node).length === 2
    return isBare ? hop?.field : undefined
}

interface LevelPartProps {
    builder: ReportBuilder
    groups: BuilderDraft['groups']
    index: number
    target: CatalogSource | undefined
}

const setLevel = (
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

function LevelFormula({
    isVisible,
    builder,
    formula,
    onChange,
    testID,
}: {
    isVisible: boolean
    builder: ReportBuilder
    formula: FormulaDraft | null
    onChange: (formula: FormulaDraft) => void
    testID: string
}) {
    if (!isVisible || !formula) return null
    return (
        <ReportFormula
            builder={builder}
            place={{ kind: 'level' }}
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
function GroupLabel({
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
function GroupFill({
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

/** "Show only groups where…": conditions on the level's totals, columns and records (R18). */
function GroupHaving({
    builder,
    groups,
    index,
    records,
}: Omit<LevelPartProps, 'target'> & { records: readonly FormulaRecord[] }) {
    const control = builder.form.control
    const source = useWatch({ control, name: 'source' })
    const columns = useWatch({ control, name: 'columns' })
    const params = useWatch({ control, name: 'params' })
    const group = groups[index]
    if (!group) return null
    // Read with useWatch, so the condition menus follow other sections (F27). A level
    // reads the records of its own group and the groups around it, not inner ones.
    const scope = levelScope({ source, columns, groups, params }, builder.catalog)
    const level = { ...scope, records }
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

interface GroupRowProps {
    builder: ReportBuilder
    groups: BuilderDraft['groups']
    index: number
    choices: Choice[]
}

function GroupRow({ builder, groups, index, choices }: GroupRowProps) {
    const { control } = builder.form
    const source = useWatch({ control, name: 'source' })
    const columns = useWatch({ control, name: 'columns' })
    const group = groups[index]
    if (!group) return null
    const { catalog } = builder
    const testID = `report-builder-group-${index}`
    const isDate = (key: string) => choices.find(choice => choice.key === key)?.isDate === true
    const options = choices.filter(choice => !choice.isNested).map(asOption)
    const draft = { source, columns }
    const target = groupTarget(draft, catalog, group)
    // A bucket left from a date column would reach the engine on a column that has
    // none, and a label or fill of another record reads fields it no longer has.
    const onColumn = (key: string) => {
        const nextTarget = groupTarget(draft, catalog, { col: key })
        const sameRecord = nextTarget?.key === target?.key
        const patch: Partial<GroupLevelDraft> = {
            col: key,
            bucket: isDate(key) ? group.bucket : '',
            ...(sameRecord ? {} : { label: null, fill: { on: false, where: emptyGroup() } }),
        }
        setLevel(builder, groups, index, patch)
    }
    const typeOf = (key: string) => choices.find(choice => choice.key === key)?.type ?? 'text'
    const above = aboveOf(groups, index, columns).map(g => ({ ...g, type: typeOf(g.col) }))
    const records = recordGroups({ source, columns, groups: groups.slice(0, index + 1) }, catalog)
    const labelField = columns.find(c => c.id === group.col)?.labelField ?? ''
    const part = { builder, groups, index, target }
    return (
        <View testID={testID} className="gap-2 rounded-lg border border-border p-2">
            <View className="flex-row items-center">
                <Text className="flex-1 text-xs text-muted-foreground">{`Level ${index + 1}`}</Text>
                <IconButton
                    label="Move level up"
                    testID={`${testID}-up`}
                    isVisible={index > 0}
                    icon={ArrowUp}
                    onPress={() => builder.setGroups(moved(groups, index, index - 1))}
                />
                <IconButton
                    label="Move level down"
                    testID={`${testID}-down`}
                    isVisible={index < groups.length - 1}
                    icon={ArrowDown}
                    onPress={() => builder.setGroups(moved(groups, index, index + 1))}
                />
                <IconButton
                    label="Remove level"
                    testID={`${testID}-remove`}
                    isVisible
                    icon={Trash2}
                    onPress={() => builder.setGroups(groups.filter((_, i) => i !== index))}
                />
            </View>
            <SelectInput
                name={`groups.${index}.col`}
                control={control}
                label="Group by"
                options={options}
                onValueChange={onColumn}
                testID={`${testID}-col`}
            />
            <BucketSelect
                isVisible={isDate(group.col)}
                name={`groups.${index}.bucket`}
                control={control}
                testID={`${testID}-bucket`}
            />
            <Toggle
                name={`groups.${index}.subtotals`}
                control={control}
                label="Subtotals"
                testID={`${testID}-subtotals`}
            />
            <GroupLabel {...part} labelField={labelField} />
            <GroupFill {...part} above={above} />
            <GroupHaving builder={builder} groups={groups} index={index} records={records} />
            <Toggle
                name={`groups.${index}.open`}
                control={control}
                label="Open when the report opens"
                testID={`${testID}-open`}
            />
        </View>
    )
}

function GroupRows({ builder }: { builder: ReportBuilder }) {
    const groups = useWatch({ control: builder.form.control, name: 'groups' })
    const choices = useColumnChoices(builder)
    const rows = groups.map((group, index) => (
        <GroupRow
            key={group.id}
            builder={builder}
            groups={groups}
            index={index}
            choices={choices}
        />
    ))
    const first = choices.find(choice => !choice.isNested)
    const add = () =>
        builder.setGroups([
            ...groups,
            {
                id: nextColumnId(
                    groups.map(g => g.id),
                    'level'
                ),
                col: first?.key ?? '',
                bucket: '',
                subtotals: true,
                open: false,
                label: null,
                having: emptyGroup(),
                fill: { on: false, where: emptyGroup() },
            },
        ])
    return (
        <View className="gap-2">
            {rows}
            <AddButton
                isVisible={groups.length < MAX_GROUP_LEVELS}
                label="Add group level"
                testID="report-builder-group-add"
                onPress={add}
            />
        </View>
    )
}

type SortKey = { kind: 'col'; col: string } | { kind: 'formula'; formula: FormulaDraft }

/** A sort key's choice: a column, a field of a group's record (R9), or a formula. */
interface SortChoice {
    id: string
    label: string
    key: SortKey
}

const FORMULA_CHOICE = ':formula'

function sortChoices(
    choices: readonly Choice[],
    records: readonly FormulaRecord[],
    catalog: Catalog
): SortChoice[] {
    const columns = choices.map(choice => ({
        id: choice.key,
        label: choice.label,
        key: { kind: 'col' as const, col: choice.key },
    }))
    const recordFields = records.flatMap(record =>
        (catalog.get(record.source)?.fields ?? [])
            .filter(f => f.key !== 'id')
            .map(f => ({
                id: `record.${record.group}.${f.key}`,
                label: `${record.label} › ${f.label}`,
                key: {
                    kind: 'formula' as const,
                    formula: {
                        kind: 'editable' as const,
                        node: { record: record.group, path: [{ field: f.key }] },
                    },
                },
            }))
    )
    const formula = { kind: 'formula' as const, formula: NEW_FORMULA }
    return [...columns, ...recordFields, { id: FORMULA_CHOICE, label: 'Formula…', key: formula }]
}

/** The choice a sort key is: its column, its record field, or a formula. */
function sortChoiceId(key: SortDraft): string {
    if (key.kind === 'col') return key.col
    const record = key.formula.node.record
    const field = record ? bareRecordField(key.formula.node, record) : undefined
    return record && field ? `record.${record}.${field}` : FORMULA_CHOICE
}

function sortChoiceLabel(key: SortDraft, offered: readonly SortChoice[]): string {
    const id = sortChoiceId(key)
    if (id === FORMULA_CHOICE) return 'Formula'
    return offered.find(c => c.id === id)?.label ?? `${id} (missing)`
}

interface SortRowProps {
    builder: ReportBuilder
    sort: BuilderDraft['sort']
    index: number
    offered: readonly SortChoice[]
}

function SortRow({ builder, sort, index, offered }: SortRowProps) {
    const { control } = builder.form
    const key = sort[index]
    if (!key) return null
    const testID = `report-builder-sort-${index}`
    const current = sortChoiceId(key)
    const replace = (next: SortKey) =>
        builder.setSort(
            sort.map((k, i) =>
                i === index ? { ...next, dir: k.dir, explicitDir: k.explicitDir } : k
            )
        )
    const items: PickItem[] = offered.map(choice => ({
        id: choice.id,
        label: choice.label,
        isSelected: choice.id === current,
        onSelect: () => {
            if (choice.id !== current) replace(choice.key)
        },
    }))
    const isFormula = current === FORMULA_CHOICE
    const formula = key.kind === 'formula' ? key.formula : null
    const error = builder.form.getFieldState(`sort.${index}.col`, builder.form.formState).error
    return (
        <View testID={testID} className="gap-1 rounded-lg border border-border p-2">
            <View className="flex-row items-center gap-2">
                <Text className="text-xs text-muted-foreground">{`Sort ${index + 1} by`}</Text>
                <View className="flex-1 flex-row">
                    <PickMenu
                        label={sortChoiceLabel(key, offered)}
                        purpose="Sort by"
                        items={items}
                        testID={`${testID}-col`}
                    />
                </View>
                <IconButton
                    label="Remove sort"
                    testID={`${testID}-remove`}
                    isVisible
                    icon={Trash2}
                    onPress={() => builder.setSort(sort.filter((_, i) => i !== index))}
                />
            </View>
            <FieldError message={error?.message} testID={`${testID}-error`} />
            <LevelFormula
                isVisible={isFormula}
                builder={builder}
                formula={formula}
                onChange={next => replace({ kind: 'formula', formula: next })}
                testID={`${testID}-formula`}
            />
            <ButtonGroup
                name={`sort.${index}.dir`}
                control={control}
                label="Order"
                horizontal
                options={DIR_OPTIONS}
            />
        </View>
    )
}

// A sort key has no id: what it sorts on and how many earlier keys sort on the
// same name it, so removing a key does not hand its row to the next one.
function sortRowKeys(sort: BuilderDraft['sort']): string[] {
    const seen = new Map<string, number>()
    return sort.map(key => {
        const on = key.kind === 'col' ? key.col : JSON.stringify(key.formula.node)
        const n = seen.get(on) ?? 0
        seen.set(on, n + 1)
        return `${on}#${n}`
    })
}

function AddSort({
    isVisible,
    offered,
    onAdd,
}: {
    isVisible: boolean
    offered: readonly SortChoice[]
    onAdd: (key: SortKey) => void
}) {
    if (!isVisible) return null
    const items: PickItem[] = offered.map(choice => ({
        id: choice.id,
        label: choice.label,
        onSelect: () => onAdd(choice.key),
    }))
    return (
        <View className="flex-row">
            <PickMenu
                label="Add sort"
                purpose="Sort by"
                items={items}
                testID="report-builder-sort-add-menu"
            />
        </View>
    )
}

function SortRows({ builder }: { builder: ReportBuilder }) {
    const control = builder.form.control
    const sort = useWatch({ control, name: 'sort' })
    const source = useWatch({ control, name: 'source' })
    const columns = useWatch({ control, name: 'columns' })
    const groups = useWatch({ control, name: 'groups' })
    const choices = useColumnChoices(builder)
    const records = recordGroups({ source, columns, groups }, builder.catalog)
    const offered = sortChoices(choices, records, builder.catalog)
    const rows = sortRowKeys(sort).map((rowKey, index) => (
        <SortRow key={rowKey} builder={builder} sort={sort} index={index} offered={offered} />
    ))
    // A new key says no direction of its own, so a save writes none for A to Z.
    const add = (key: SortKey) =>
        builder.setSort([...sort, { ...key, dir: 'asc', explicitDir: false }])
    return (
        <View className="gap-2">
            {rows}
            <AddSort isVisible={sort.length < MAX_SORT_KEYS} offered={offered} onAdd={add} />
        </View>
    )
}

export function GroupSortSection({
    builder,
    problems,
}: {
    builder: ReportBuilder
    problems: Problem[]
}) {
    return (
        <BuilderSection title="Group and sort" section="group" problems={problems}>
            <Text className="text-xs font-semibold uppercase text-muted-foreground">Groups</Text>
            <GroupRows builder={builder} />
            <Text className="text-xs font-semibold uppercase text-muted-foreground">Sort</Text>
            <SortRows builder={builder} />
        </BuilderSection>
    )
}
