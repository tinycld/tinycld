import { ButtonGroup, useWatch } from '@tinycld/core/ui/form'
import { type PickItem, PickMenu } from '@tinycld/core/ui/pick-menu'
import { Trash2 } from 'lucide-react-native'
import { Text, View } from 'react-native'
import type { ReportBuilder } from '../../hooks/useReportBuilder'
import { type Catalog, MAX_SORT_KEYS } from '../../lib/builder/columns'
import type { BuilderDraft, FormulaDraft, SortDraft } from '../../lib/builder/draft'
import {
    type ColumnChoice as Choice,
    recordGroups,
    windowColumns,
} from '../../lib/builder/formula-context'
import type { FormulaRecord } from '../../lib/builder/formula-parts'
import { bareRecordField, LevelFormula } from './GroupLevelParts'
import { IconButton, NEW_FORMULA } from './parts'
import { useColumnChoices } from './report-formula'

// The report's sort keys: a column, a field of a group's record (R9), or a formula.
// A running value or a percent is not a sort key (run.go notASortKey).

const DIR_OPTIONS = [
    { value: 'asc', label: 'A to Z' },
    { value: 'desc', label: 'Z to A' },
]

function FieldError({ message, testID }: { message: string | undefined; testID: string }) {
    if (!message) return null
    return (
        <Text testID={testID} role="alert" className="text-xs text-danger">
            {message}
        </Text>
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
    windows: ReadonlySet<string>,
    records: readonly FormulaRecord[],
    catalog: Catalog
): SortChoice[] {
    const columns = choices
        .filter(choice => !windows.has(choice.key))
        .map(choice => ({
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
                place={{ kind: 'level' }}
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
                onValueChange={builder.validateLists}
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

export function SortRows({ builder }: { builder: ReportBuilder }) {
    const control = builder.form.control
    const sort = useWatch({ control, name: 'sort' })
    const source = useWatch({ control, name: 'source' })
    const columns = useWatch({ control, name: 'columns' })
    const groups = useWatch({ control, name: 'groups' })
    const choices = useColumnChoices(builder)
    const records = recordGroups({ source, columns, groups }, builder.catalog)
    const offered = sortChoices(choices, windowColumns(columns), records, builder.catalog)
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
