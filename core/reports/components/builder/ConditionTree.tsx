import { Button, ButtonText } from '@tinycld/core/ui/button'
import { useWatch } from '@tinycld/core/ui/form'
import { type PickItem, PickMenu } from '@tinycld/core/ui/pick-menu'
import { Trash2 } from 'lucide-react-native'
import { Text, View } from 'react-native'
import type { ReportBuilder } from '../../hooks/useReportBuilder'
import { sourceAtLinks } from '../../lib/builder/columns'
import {
    addFilterItem,
    type ConditionDraft,
    emptyGroup,
    type FilterItem,
    type FilterPath,
    filterItemKeys,
    type GroupDraft,
    type LevelScope,
    needsValue,
    opLabel,
    opsShownFor,
    type RelatedDraft,
    removeFilterItem,
    setMatchAt,
    updateFilterItem,
} from '../../lib/builder/filter'
import type { ParamDef } from '../../lib/spec'
import {
    ColumnSubject,
    FieldSubject,
    LevelAdders,
    RecordSubject,
    RowAdders,
    TotalSubject,
} from './ConditionSubjects'
import { AboveMenu, AskToggle, ConditionValue, TotalPlaces } from './ConditionValues'
import {
    type AboveChoice,
    conditionInfo,
    type FilterContext,
    itemTestId,
    setCondition,
} from './condition-context'
import { IconButton } from './parts'

// A tree of conditions joined by All of / Any of / None of: the report filter, a
// group level's "Show only groups where…" (having, R18) and a fill's conditions on
// the linked records (R6). On a level a condition compares a total of a field, a
// column or a field of a group's record; in a fill under other groups its value can
// be the key of a group above (F21). A row's subject is picked in
// ConditionSubjects.tsx, its value in ConditionValues.tsx.

export type { AboveChoice } from './condition-context'

const MATCHES = [
    { value: 'all', label: 'All of' },
    { value: 'any', label: 'Any of' },
    { value: 'none', label: 'None of' },
] as const

export interface ConditionTreeProps {
    builder: ReportBuilder
    root: GroupDraft
    /** The whole tree changed; `added` are the params a condition newly asks for. */
    onChange(next: GroupDraft, ...added: ParamDef[]): void
    /** The source the conditions' fields are on. */
    baseKey: string
    /** The test id stem: report-builder-filter, report-builder-group-0-having, … */
    testID: string
    /** On a group level the conditions read totals, columns and group records (R18). */
    level?: LevelScope
    /** The group level's own id: its record's adder is …-add-record. */
    group?: string
    /** In a fill under other groups: the groups above whose key a value can be (F21). */
    above?: readonly AboveChoice[]
    /** What the form refuses before a run, so a blocked Save says why. */
    error?: string
}

interface RowProps<T> {
    ctx: FilterContext
    item: T
    path: FilterPath
    baseKey: string
}

// A removed item takes the values its conditions asked for with it (ctx.change).
function RemoveButton({
    ctx,
    path,
    label,
}: {
    ctx: FilterContext
    path: FilterPath
    label: string
}) {
    return (
        <IconButton
            label={label}
            testID={`${itemTestId(ctx, path)}-remove`}
            isVisible
            icon={Trash2}
            onPress={() => ctx.change(removeFilterItem(ctx.root, path))}
        />
    )
}

const MISSING_MESSAGES = {
    field: 'A condition names a field that is no longer offered.',
    total: 'A condition names a field that is no longer offered.',
    column: 'A condition names a column that is no longer in the report.',
    record: 'A condition names a group that no longer links to a record.',
    link: 'A related-rows condition names a link that is no longer offered.',
}

function MissingRow({
    ctx,
    path,
    missing,
}: {
    ctx: FilterContext
    path: FilterPath
    missing: keyof typeof MISSING_MESSAGES
}) {
    return (
        <View
            testID={itemTestId(ctx, path)}
            className="flex-row items-center gap-2 rounded-md border border-border p-2"
        >
            <Text className="flex-1 text-sm text-muted-foreground">
                {MISSING_MESSAGES[missing]}
            </Text>
            <RemoveButton ctx={ctx} path={path} label="Remove condition" />
        </View>
    )
}

function ConditionRow({ ctx, item, path, baseKey }: RowProps<ConditionDraft>) {
    const id = itemTestId(ctx, path)
    const info = conditionInfo(ctx, item, baseKey)
    if (!info) return <MissingRow ctx={ctx} path={path} missing={item.subject} />
    const type = info.field.type
    // A new operator starts without an asked value: the param's kind follows the
    // operator. Is any of, and an operator with no value, compare with no group above.
    const opItems: PickItem[] = opsShownFor(type, item.op).map(op => ({
        id: op,
        label: opLabel(op, type),
        isSelected: op === item.op,
        onSelect: () => {
            if (op === item.op) return
            const above = op === 'in' || !needsValue(op) ? '' : item.above
            setCondition(ctx, path, { op, param: '', paramHigh: '', above })
        },
    }))
    const subject = { ctx, item, path, baseKey, info }
    const value = { ctx, item, path, info }
    return (
        <View testID={id} className="gap-1.5 rounded-md border border-border p-2">
            <View className="flex-row flex-wrap items-center gap-1.5">
                <FieldSubject {...subject} />
                <TotalSubject {...subject} />
                <ColumnSubject {...subject} />
                <RecordSubject {...subject} />
                <PickMenu
                    label={opLabel(item.op, type)}
                    purpose="Condition"
                    items={opItems}
                    testID={`${id}-op`}
                />
                <RemoveButton ctx={ctx} path={path} label="Remove condition" />
            </View>
            <TotalPlaces {...value} />
            <AboveMenu {...value} />
            <ConditionValue {...value} />
            <AskToggle {...value} />
        </View>
    )
}

function setHas(ctx: FilterContext, path: FilterPath, has: boolean) {
    ctx.change(
        updateFilterItem(ctx.root, path, item =>
            item.kind === 'related' ? { ...item, has } : item
        )
    )
}

// "Has / has no related <rows>" over a to-many link; its conditions are on the rows
// the link reaches, which have no level and no group above.
function RelatedRow({ ctx, item, path, baseKey }: RowProps<RelatedDraft>) {
    const id = itemTestId(ctx, path)
    const target = sourceAtLinks(ctx.catalog, baseKey, item.link)
    if (!target) return <MissingRow ctx={ctx} path={path} missing="link" />
    const link = ctx.catalog.get(baseKey)?.links.find(l => l.key === item.link[0]?.link)
    const hasItems: PickItem[] = [
        { id: 'yes', label: 'Has', isSelected: item.has, onSelect: () => setHas(ctx, path, true) },
        {
            id: 'no',
            label: 'Has no',
            isSelected: !item.has,
            onSelect: () => setHas(ctx, path, false),
        },
    ]
    const hasLabel = item.has ? 'Has' : 'Has no'
    const inner: FilterContext = { ...ctx, level: undefined, group: '', above: [] }
    return (
        <View testID={id} className="gap-1.5 rounded-md border border-border p-2">
            <View className="flex-row flex-wrap items-center gap-1.5">
                <PickMenu
                    label={hasLabel}
                    purpose="Related rows"
                    items={hasItems}
                    testID={`${id}-has`}
                />
                <Text className="flex-1 text-sm text-foreground">
                    {`related ${link?.label ?? 'rows'}`}
                </Text>
                <RemoveButton ctx={ctx} path={path} label="Remove related rows" />
            </View>
            <GroupBody ctx={inner} group={item.where} path={path} baseKey={target.key} />
        </View>
    )
}

function FixedRow({ ctx, path }: { ctx: FilterContext; path: FilterPath }) {
    return (
        <View
            testID={itemTestId(ctx, path)}
            className="flex-row items-center gap-2 rounded-md border border-border p-2"
        >
            <Text className="flex-1 text-sm text-muted-foreground">
                A condition the builder cannot edit. It is kept as it is.
            </Text>
            <RemoveButton ctx={ctx} path={path} label="Remove condition" />
        </View>
    )
}

function NestedGroup({ ctx, item, path, baseKey }: RowProps<GroupDraft>) {
    return (
        <View
            testID={itemTestId(ctx, path)}
            className="gap-1.5 rounded-md border border-border p-2"
        >
            <View className="flex-row justify-end">
                <RemoveButton ctx={ctx} path={path} label="Remove group" />
            </View>
            <GroupBody ctx={ctx} group={item} path={path} baseKey={baseKey} />
        </View>
    )
}

function ItemEditor({ ctx, item, path, baseKey }: RowProps<FilterItem>) {
    if (item.kind === 'condition')
        return <ConditionRow ctx={ctx} item={item} path={path} baseKey={baseKey} />
    if (item.kind === 'related')
        return <RelatedRow ctx={ctx} item={item} path={path} baseKey={baseKey} />
    if (item.kind === 'fixed') return <FixedRow ctx={ctx} path={path} />
    return <NestedGroup ctx={ctx} item={item} path={path} baseKey={baseKey} />
}

function GroupBody({
    ctx,
    group,
    path,
    baseKey,
}: {
    ctx: FilterContext
    group: GroupDraft
    path: FilterPath
    baseKey: string
}) {
    const id = itemTestId(ctx, path)
    const matchItems: PickItem[] = MATCHES.map(m => ({
        id: m.value,
        label: m.label,
        isSelected: m.value === group.match,
        onSelect: () => ctx.change(setMatchAt(ctx.root, path, m.value)),
    }))
    const matchLabel = MATCHES.find(m => m.value === group.match)?.label ?? 'All of'
    const onGroup = () =>
        ctx.change(addFilterItem(ctx.root, path, { ...emptyGroup(), match: 'any' }))
    const keys = filterItemKeys(group.items)
    const items = group.items.map((item, i) => (
        <ItemEditor key={keys[i]} ctx={ctx} item={item} path={[...path, i]} baseKey={baseKey} />
    ))
    return (
        <View testID={`${id}-group`} className="gap-1.5">
            <PickMenu
                label={matchLabel}
                purpose="Join conditions"
                items={matchItems}
                testID={`${id}-match`}
            />
            {items}
            <View className="flex-row flex-wrap gap-1.5">
                <RowAdders ctx={ctx} path={path} baseKey={baseKey} />
                <LevelAdders ctx={ctx} path={path} baseKey={baseKey} />
                <Button testID={`${id}-add-group`} variant="outline" size="sm" onPress={onGroup}>
                    <ButtonText>Add group</ButtonText>
                </Button>
            </View>
        </View>
    )
}

function TreeError({ message, testID }: { message: string | undefined; testID: string }) {
    if (!message) return null
    return (
        <Text testID={testID} role="alert" className="text-xs text-danger">
            {message}
        </Text>
    )
}

/** A condition tree's editor: the filter, a level's having, or a fill's conditions. */
export function ConditionTree({
    builder,
    root,
    onChange,
    baseKey,
    testID,
    level,
    group = '',
    above = [],
    error,
}: ConditionTreeProps) {
    const params = useWatch({ control: builder.form.control, name: 'params' })
    const ctx: FilterContext = {
        builder,
        catalog: builder.catalog,
        root,
        params,
        testID,
        level,
        group,
        above,
        change: onChange,
    }
    return (
        <View className="gap-1.5">
            <TreeError message={error} testID={`${testID}-error`} />
            <View testID={testID}>
                <GroupBody ctx={ctx} group={root} path={[]} baseKey={baseKey} />
            </View>
        </View>
    )
}
