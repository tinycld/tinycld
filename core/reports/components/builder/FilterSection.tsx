import { Button, ButtonText } from '@tinycld/core/ui/button'
import { useWatch } from '@tinycld/core/ui/form'
import { DateField } from '@tinycld/core/ui/forms/fields/DateField'
import { PlainInput } from '@tinycld/core/ui/PlainInput'
import { type PickItem, PickMenu } from '@tinycld/core/ui/pick-menu'
import { Switch } from '@tinycld/core/ui/switch'
import { Trash2 } from 'lucide-react-native'
import { Pressable, Text, View } from 'react-native'
import type { ReportBuilder } from '../../hooks/useReportBuilder'
import {
    type Catalog,
    columnHint,
    describePath,
    MAX_PARAMS,
    nextColumnId,
    type PathInfo,
    pathLabel,
    sourceAtLinks,
} from '../../lib/builder/columns'
import { paramsReadByColumns } from '../../lib/builder/draft'
import {
    addFilterItem,
    askParamFor,
    type ConditionDraft,
    emptyCondition,
    emptyGroup,
    type FilterItem,
    type FilterPath,
    filterItemKeys,
    filterParams,
    type GroupDraft,
    needsValue,
    opLabel,
    opsFor,
    opsShownFor,
    type RelatedDraft,
    removeFilterItem,
    setMatchAt,
    updateFilterItem,
} from '../../lib/builder/filter'
import { isDateParam, PRESET_LABELS, presetsFor } from '../../lib/params'
import type { ParamDef, SpecHop } from '../../lib/spec'
import type { Problem } from '../../lib/types'
import { REPORT_TAP_TARGET } from '../ReportHeader'
import { BuilderSection } from './BuilderSection'
import { ColumnPicker } from './ColumnPicker'
import { IconButton } from './parts'

const MATCHES = [
    { value: 'all', label: 'All of' },
    { value: 'any', label: 'Any of' },
    { value: 'none', label: 'None of' },
] as const

function filterTestId(path: FilterPath): string {
    return path.length === 0 ? 'report-builder-filter' : `report-builder-filter-${path.join('-')}`
}

// What every part of the filter editor needs: the tree, the asked params, and how
// to change them. Every change replaces the whole filter and its params in the form
// in one edit: a param no condition asks for any more leaves with it, and `added`
// is a newly asked one.
interface FilterContext {
    builder: ReportBuilder
    catalog: Catalog
    root: GroupDraft
    params: ParamDef[]
    change(next: GroupDraft, added?: ParamDef): void
}

function setCondition(
    ctx: FilterContext,
    path: FilterPath,
    patch: Partial<ConditionDraft>,
    added?: ParamDef
) {
    ctx.change(
        updateFilterItem(ctx.root, path, item =>
            item.kind === 'condition' ? { ...item, ...patch } : item
        ),
        added
    )
}

/** A new condition on a field starts with the field type's first operator. */
function newCondition(ctx: FilterContext, baseKey: string, path: SpecHop[]): ConditionDraft {
    const type = describePath(ctx.catalog, baseKey, path)?.field.type ?? 'text'
    return { ...emptyCondition(path), op: opsFor(type)[0] ?? 'eq' }
}

interface ValueProps {
    ctx: FilterContext
    item: ConditionDraft
    path: FilterPath
    info: PathInfo
}

function ChoiceChip({
    label,
    isOn,
    testID,
    onPress,
}: {
    label: string
    isOn: boolean
    testID: string
    onPress: () => void
}) {
    const chipClass = isOn ? 'border-primary bg-primary' : 'border-border'
    const textClass = isOn ? 'text-primary-foreground' : 'text-foreground'
    return (
        <Pressable
            testID={testID}
            accessibilityRole="checkbox"
            aria-checked={isOn}
            onPress={onPress}
            className={`justify-center rounded-full border px-3 ${chipClass}`}
            style={{ minHeight: REPORT_TAP_TARGET }}
        >
            <Text className={`text-sm ${textClass}`}>{label}</Text>
        </Pressable>
    )
}

// "Is any of": each option is a chip that is on or off.
function ChoiceValues({ ctx, item, path, info }: ValueProps) {
    const id = filterTestId(path)
    const toggle = (value: string) => {
        const values = item.values.includes(value)
            ? item.values.filter(v => v !== value)
            : [...item.values, value]
        setCondition(ctx, path, { values })
    }
    const chips = (info.field.options ?? []).map(option => (
        <ChoiceChip
            key={option.value}
            label={option.label}
            isOn={item.values.includes(option.value)}
            testID={`${id}-value-${option.value}`}
            onPress={() => toggle(option.value)}
        />
    ))
    return <View className="flex-row flex-wrap gap-1.5">{chips}</View>
}

function ChoiceValue({ ctx, item, path, info }: ValueProps) {
    const options = info.field.options ?? []
    const items: PickItem[] = options.map(option => ({
        id: option.value,
        label: option.label,
        isSelected: option.value === item.value,
        onSelect: () => setCondition(ctx, path, { value: option.value }),
    }))
    const label = options.find(o => o.value === item.value)?.label ?? 'Choose'
    return (
        <PickMenu
            label={label}
            purpose={`${info.field.label} value`}
            items={items}
            testID={`${filterTestId(path)}-value`}
            size="touch"
        />
    )
}

const NUMERIC_TYPES = new Set(['number', 'money', 'percent'])

function OneValue({ ctx, item, path, info, part }: ValueProps & { part: 'value' | 'high' }) {
    const testID = `${filterTestId(path)}-${part}`
    const label = part === 'high' ? `${info.field.label}, to` : `${info.field.label} value`
    const onChange = (text: string | null) => setCondition(ctx, path, { [part]: text ?? '' })
    const type = info.field.type
    if (type === 'date' || type === 'datetime') {
        return (
            <DateField
                value={item[part] || null}
                onChange={onChange}
                withTime={false}
                accessibilityLabel={label}
                testID={testID}
            />
        )
    }
    const keyboardType = NUMERIC_TYPES.has(type) ? 'decimal-pad' : 'default'
    return (
        <PlainInput
            value={item[part]}
            onChangeText={onChange}
            accessibilityLabel={label}
            keyboardType={keyboardType}
            testID={testID}
            className="min-w-24 flex-1 rounded-md border border-border px-2 py-2 text-sm text-foreground"
        />
    )
}

function HighValue({ isVisible, ...props }: ValueProps & { isVisible: boolean }) {
    if (!isVisible) return null
    return <OneValue {...props} part="high" />
}

// An asked value is set by the reader, so the condition shows no value of its own.
function ConditionValue(props: ValueProps) {
    const { item, info } = props
    if (!needsValue(item.op) || item.param) return null
    if (info.field.type === 'select' && item.op === 'in') return <ChoiceValues {...props} />
    if (info.field.type === 'select') return <ChoiceValue {...props} />
    return (
        <View className="flex-row flex-wrap items-center gap-1.5">
            <OneValue {...props} part="value" />
            <HighValue {...props} isVisible={item.op === 'between'} />
        </View>
    )
}

function presetOf(param: ParamDef): string {
    const value = param.default
    if (typeof value !== 'object' || value === null || !('preset' in value)) return ''
    return String(value.preset)
}

// A date asked when run may start on a preset (This month, End of last month, …).
function DefaultPreset({ ctx, item, path }: ValueProps) {
    const param = ctx.params.find(p => p.id === item.param)
    if (!param || !isDateParam(param)) return null
    const current = presetOf(param)
    const setDefault = (preset: string) => {
        const next = { ...param, default: preset ? { preset } : undefined }
        ctx.builder.setParams(ctx.params.map(p => (p.id === param.id ? next : p)))
    }
    const items: PickItem[] = [
        {
            id: 'none',
            label: 'No default',
            isSelected: current === '',
            onSelect: () => setDefault(''),
        },
        ...presetsFor(param).map(preset => ({
            id: preset,
            label: PRESET_LABELS[preset],
            isSelected: preset === current,
            onSelect: () => setDefault(preset),
        })),
    ]
    const chosen = items.find(i => i.id === current && current !== '')
    const label = chosen ? `Default: ${chosen.label}` : 'No default'
    return (
        <PickMenu
            label={label}
            purpose="Default period"
            items={items}
            testID={`${filterTestId(path)}-default`}
        />
    )
}

// "Ask when run" turns the value into a param the reader sets from a chip (R16).
// Money and yes/no cannot be asked. A read-back condition that already asks stays
// switchable off even when its operator could not be asked today.
function AskToggle(props: ValueProps) {
    const { ctx, item, path, info } = props
    const isAsking = item.param !== ''
    const canAsk = askParamFor(info.field, item.op, 'probe') !== null
    if (!isAsking && !canAsk) return null
    const isFull = !isAsking && ctx.params.length >= MAX_PARAMS
    const onChange = (on: boolean) => {
        if (!on) {
            setCondition(ctx, path, { param: '' })
            return
        }
        const id = nextColumnId(
            ctx.params.map(p => p.id),
            columnHint(item.path)
        )
        const def = askParamFor(info.field, item.op, id)
        if (def) setCondition(ctx, path, { param: def.id }, def)
    }
    return (
        <View className="flex-row flex-wrap items-center gap-2">
            <Switch
                value={isAsking}
                onValueChange={onChange}
                isDisabled={isFull}
                accessibilityLabel="Ask when the report runs"
                testID={`${filterTestId(path)}-ask`}
            />
            <Text className="text-sm text-foreground">Ask when the report runs</Text>
            <DefaultPreset {...props} />
        </View>
    )
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
            testID={`${filterTestId(path)}-remove`}
            isVisible
            icon={Trash2}
            onPress={() => ctx.change(removeFilterItem(ctx.root, path))}
        />
    )
}

const MISSING_MESSAGES = {
    field: 'A condition names a field that is no longer offered.',
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
            testID={filterTestId(path)}
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
    const id = filterTestId(path)
    const info = describePath(ctx.catalog, baseKey, item.path)
    if (!info) return <MissingRow ctx={ctx} path={path} missing="field" />
    const type = info.field.type
    // A new operator starts without an asked value: the param's kind follows the operator.
    const opItems: PickItem[] = opsShownFor(type, item.op).map(op => ({
        id: op,
        label: opLabel(op, type),
        isSelected: op === item.op,
        onSelect: () => {
            if (op !== item.op) setCondition(ctx, path, { op, param: '' })
        },
    }))
    const onField = (next: SpecHop[]) =>
        ctx.change(updateFilterItem(ctx.root, path, () => newCondition(ctx, baseKey, next)))
    return (
        <View testID={id} className="gap-1.5 rounded-md border border-border p-2">
            <View className="flex-row flex-wrap items-center gap-1.5">
                <ColumnPicker
                    catalog={ctx.catalog}
                    baseKey={baseKey}
                    onPick={onField}
                    label={pathLabel(info)}
                    testID={`${id}-field`}
                    toOneOnly
                />
                <PickMenu
                    label={opLabel(item.op, type)}
                    purpose="Condition"
                    items={opItems}
                    testID={`${id}-op`}
                />
                <RemoveButton ctx={ctx} path={path} label="Remove condition" />
            </View>
            <ConditionValue ctx={ctx} item={item} path={path} info={info} />
            <AskToggle ctx={ctx} item={item} path={path} info={info} />
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
// the link reaches.
function RelatedRow({ ctx, item, path, baseKey }: RowProps<RelatedDraft>) {
    const id = filterTestId(path)
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
            <GroupBody ctx={ctx} group={item.where} path={path} baseKey={target.key} />
        </View>
    )
}

function FixedRow({ ctx, path }: { ctx: FilterContext; path: FilterPath }) {
    return (
        <View
            testID={filterTestId(path)}
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
        <View testID={filterTestId(path)} className="gap-1.5 rounded-md border border-border p-2">
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

function AddRelated({
    ctx,
    path,
    baseKey,
}: {
    ctx: FilterContext
    path: FilterPath
    baseKey: string
}) {
    const manyLinks = (ctx.catalog.get(baseKey)?.links ?? []).filter(
        l => l.cardinality === 'toMany'
    )
    if (manyLinks.length === 0) return null
    const items: PickItem[] = manyLinks.map(link => ({
        id: link.key,
        label: link.label,
        onSelect: () =>
            ctx.change(
                addFilterItem(ctx.root, path, {
                    kind: 'related',
                    has: true,
                    link: [{ link: link.key }],
                    where: emptyGroup(),
                })
            ),
    }))
    return (
        <PickMenu
            label="Add related rows"
            purpose="Related rows"
            items={items}
            testID={`${filterTestId(path)}-add-related`}
        />
    )
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
    const id = filterTestId(path)
    const matchItems: PickItem[] = MATCHES.map(m => ({
        id: m.value,
        label: m.label,
        isSelected: m.value === group.match,
        onSelect: () => ctx.change(setMatchAt(ctx.root, path, m.value)),
    }))
    const matchLabel = MATCHES.find(m => m.value === group.match)?.label ?? 'All of'
    const onCondition = (next: SpecHop[]) =>
        ctx.change(addFilterItem(ctx.root, path, newCondition(ctx, baseKey, next)))
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
                <ColumnPicker
                    catalog={ctx.catalog}
                    baseKey={baseKey}
                    onPick={onCondition}
                    label="Add condition"
                    testID={`${id}-add-condition`}
                    toOneOnly
                />
                <AddRelated ctx={ctx} path={path} baseKey={baseKey} />
                <Button testID={`${id}-add-group`} variant="outline" size="sm" onPress={onGroup}>
                    <ButtonText>Add group</ButtonText>
                </Button>
            </View>
        </View>
    )
}

// What the form refuses before a run (a value to enter); the server's refusals
// show above it, in the section's problems.
function FilterError({ message }: { message: string | undefined }) {
    if (!message) return null
    return (
        <Text testID="report-builder-filter-error" role="alert" className="text-xs text-danger">
            {message}
        </Text>
    )
}

export function FilterSection({
    builder,
    problems,
}: {
    builder: ReportBuilder
    problems: Problem[]
}) {
    const { control } = builder.form
    const root = useWatch({ control, name: 'filter' })
    const params = useWatch({ control, name: 'params' })
    const columns = useWatch({ control, name: 'columns' })
    const error = builder.form.getFieldState('filter', builder.form.formState).error?.message
    const ctx: FilterContext = {
        builder,
        catalog: builder.catalog,
        root,
        params,
        change: (next, added) =>
            builder.setFilter(
                next,
                filterParams(params, next, paramsReadByColumns(columns), added)
            ),
    }
    return (
        <BuilderSection title="Filter" section="filter" problems={problems}>
            <FilterError message={error} />
            <View testID="report-builder-filter">
                <GroupBody ctx={ctx} group={root} path={[]} baseKey={builder.base.key} />
            </View>
        </BuilderSection>
    )
}
