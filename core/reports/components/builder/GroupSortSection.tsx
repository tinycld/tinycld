import { Button, ButtonText } from '@tinycld/core/ui/button'
import { SelectInput, Toggle, useWatch } from '@tinycld/core/ui/form'
import { ArrowDown, ArrowUp, Trash2 } from 'lucide-react-native'
import { Text, View } from 'react-native'
import type { ReportBuilder } from '../../hooks/useReportBuilder'
import { describePath, MAX_GROUP_LEVELS, nextColumnId } from '../../lib/builder/columns'
import { type BuilderDraft, type GroupLevelDraft, groupTarget } from '../../lib/builder/draft'
import { aboveOf, emptyGroup } from '../../lib/builder/filter'
import { type ColumnChoice as Choice, readsColumn } from '../../lib/builder/formula-context'
import type { Problem } from '../../lib/types'
import { BuilderSection } from './BuilderSection'
import { moved } from './ColumnsSection'
import type { AboveChoice } from './ConditionTree'
import { GroupFill, GroupHaving, GroupLabel, setLevel } from './GroupLevelParts'
import { BucketSelect, IconButton } from './parts'
import { useColumnChoices } from './report-formula'
import { SortRows } from './SortRows'

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
    const choiceOf = (key: string) => choices.find(choice => choice.key === key)
    const isDate = (key: string) => choiceOf(key)?.isDate === true
    const options = choices.filter(choice => !choice.isNested).map(asOption)
    const draft = { source, columns }
    const target = groupTarget(draft, catalog, group)
    // A bucket left from a date column would reach the engine on a column that has
    // none; a label or fill of another record reads fields it no longer has, and a
    // label reading the old column reads a value the level no longer has.
    const onColumn = (key: string) => {
        const nextTarget = groupTarget(draft, catalog, { col: key })
        const sameRecord = nextTarget?.key === target?.key
        const dropsLabel = !sameRecord || readsColumn(group.label?.node, group.col)
        const patch: Partial<GroupLevelDraft> = {
            col: key,
            bucket: isDate(key) ? group.bucket : '',
            ...(dropsLabel ? { label: null } : {}),
            ...(sameRecord ? {} : { fill: { on: false, where: emptyGroup() } }),
        }
        setLevel(builder, groups, index, patch)
    }
    // A group above is typed as its column's cells: a relation by the source it
    // points at, a select by its options.
    const above: AboveChoice[] = aboveOf(groups, index, columns).map(g => {
        const choice = choiceOf(g.col)
        const column = columns.find(c => c.id === g.col)
        const info = column?.kind === 'path' ? describePath(catalog, source, column.path) : null
        const options = choice?.options.map(o => o.value)
        return { ...g, type: choice?.type ?? 'text', target: info?.target?.key, options }
    })
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
                onValueChange={builder.validateLists}
            />
            <Toggle
                name={`groups.${index}.subtotals`}
                control={control}
                label="Subtotals"
                testID={`${testID}-subtotals`}
                onValueChange={builder.validateLists}
            />
            <GroupLabel {...part} labelField={labelField} />
            <GroupFill {...part} above={above} />
            <GroupHaving builder={builder} groups={groups} index={index} />
            <Toggle
                name={`groups.${index}.open`}
                control={control}
                label="Open when the report opens"
                testID={`${testID}-open`}
                onValueChange={builder.validateLists}
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
