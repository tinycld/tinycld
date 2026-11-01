import { Button, ButtonText } from '@tinycld/core/ui/button'
import { ButtonGroup, SelectInput, Toggle, useWatch } from '@tinycld/core/ui/form'
import { ArrowDown, ArrowUp, Trash2 } from 'lucide-react-native'
import { Text, View } from 'react-native'
import type { ReportBuilder } from '../../hooks/useReportBuilder'
import { MAX_GROUP_LEVELS, MAX_SORT_KEYS, nextColumnId } from '../../lib/builder/columns'
import { type BuilderDraft, columnChoices } from '../../lib/builder/draft'
import type { Problem } from '../../lib/types'
import { BuilderSection } from './BuilderSection'
import { moved } from './ColumnsSection'
import { BucketSelect, IconButton } from './parts'

const DIR_OPTIONS = [
    { value: 'asc', label: 'A to Z' },
    { value: 'desc', label: 'Z to A' },
]

type Choice = ReturnType<typeof columnChoices>[number]

function useChoices(builder: ReportBuilder) {
    const columns = useWatch({ control: builder.form.control, name: 'columns' })
    return columnChoices({ source: builder.base.key, columns }, builder.catalog)
}

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
    const { control, setValue } = builder.form
    const group = groups[index]
    if (!group) return null
    const testID = `report-builder-group-${index}`
    const isDate = (key: string) => choices.find(choice => choice.key === key)?.isDate === true
    const options = choices.filter(choice => !choice.isNested).map(asOption)
    // A bucket left from a date column would reach the engine on a column that has none.
    const onColumn = (key: string) => {
        if (!isDate(key)) setValue(`groups.${index}.bucket`, '', { shouldDirty: true })
    }
    return (
        <View testID={testID} className="gap-1 rounded-lg border border-border p-2">
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
        </View>
    )
}

function GroupRows({ builder }: { builder: ReportBuilder }) {
    const groups = useWatch({ control: builder.form.control, name: 'groups' })
    const choices = useChoices(builder)
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

interface SortRowProps {
    builder: ReportBuilder
    sort: BuilderDraft['sort']
    index: number
    options: { value: string; label: string }[]
}

function SortRow({ builder, sort, index, options }: SortRowProps) {
    const { control } = builder.form
    const testID = `report-builder-sort-${index}`
    return (
        <View testID={testID} className="gap-1 rounded-lg border border-border p-2">
            <View className="flex-row items-center">
                <Text className="flex-1 text-xs text-muted-foreground">{`Sort ${index + 1}`}</Text>
                <IconButton
                    label="Remove sort"
                    testID={`${testID}-remove`}
                    isVisible
                    icon={Trash2}
                    onPress={() => builder.setSort(sort.filter((_, i) => i !== index))}
                />
            </View>
            <SelectInput
                name={`sort.${index}.col`}
                control={control}
                label="Sort by"
                options={options}
                testID={`${testID}-col`}
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

// A sort key has no id: its column and how many earlier keys sort on the same
// column name it, so removing a key does not hand its row to the next one.
function sortRowKeys(sort: BuilderDraft['sort']): string[] {
    const seen = new Map<string, number>()
    return sort.map(key => {
        const n = seen.get(key.col) ?? 0
        seen.set(key.col, n + 1)
        return `${key.col}#${n}`
    })
}

function SortRows({ builder }: { builder: ReportBuilder }) {
    const sort = useWatch({ control: builder.form.control, name: 'sort' })
    const options = useChoices(builder).map(asOption)
    const rows = sortRowKeys(sort).map((rowKey, index) => (
        <SortRow key={rowKey} builder={builder} sort={sort} index={index} options={options} />
    ))
    const add = () => builder.setSort([...sort, { col: options[0]?.value ?? '', dir: 'asc' }])
    return (
        <View className="gap-2">
            {rows}
            <AddButton
                isVisible={sort.length < MAX_SORT_KEYS}
                label="Add sort"
                testID="report-builder-sort-add"
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
