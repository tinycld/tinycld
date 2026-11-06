import { FormulaPlacesPick } from '@tinycld/core/components/forms-builder/FormulaEditor'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { TextInput } from '@tinycld/core/ui/form'
import { DateField } from '@tinycld/core/ui/forms/fields/DateField'
import { PlainInput } from '@tinycld/core/ui/PlainInput'
import { type PickItem, PickMenu } from '@tinycld/core/ui/pick-menu'
import { Switch } from '@tinycld/core/ui/switch'
import { X } from 'lucide-react-native'
import { Pressable, Text, View } from 'react-native'
import { useParamOptionLabels } from '../../hooks/useParamOptions'
import {
    avgScaleFor,
    columnHint,
    describePath,
    MAX_LABEL,
    MAX_PARAMS,
    nextColumnId,
    pathLabel,
} from '../../lib/builder/columns'
import {
    askHighParamFor,
    askParamFor,
    type ConditionDraft,
    type FilterPath,
    groupOrderChoices,
    needsValue,
    tidyParam,
    withOptionsGroup,
} from '../../lib/builder/filter'
import { BLANK_LINK_LABEL } from '../../lib/format'
import {
    type ChoiceLabels,
    isDateParam,
    type ParamValue,
    PRESET_LABELS,
    presetsFor,
} from '../../lib/params'
import {
    CURRENT_SPEC_VERSION,
    GROUP_LABEL_TOKEN,
    type GroupOrder,
    type GroupOrderBy,
    type OptionsFrom,
    type ParamDef,
    type ReportSpec,
    type SpecHop,
} from '../../lib/spec'
import type { CatalogSource } from '../../lib/types'
import { useReportUIStore } from '../../stores/report-ui-store'
import { SourcePicker } from '../ParamPickers'
import { REPORT_TAP_TARGET } from '../ReportHeader'
import { ColumnPicker } from './ColumnPicker'
import {
    type AboveChoice,
    type ConditionInfo,
    type FilterContext,
    itemTestId,
    setCondition,
} from './condition-context'
import { FieldError, IconButton } from './parts'

// The value side of a condition row: the value (a choice, chips, a text, a number
// or a date, or a range), Ask when run with a date's default, the group above a
// fill's value can be, and an average's places.

export interface ValueProps {
    ctx: FilterContext
    item: ConditionDraft
    path: FilterPath
    info: ConditionInfo
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
    const id = itemTestId(ctx, path)
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
            purpose={`${info.label} value`}
            items={items}
            testID={`${itemTestId(ctx, path)}-value`}
            size="touch"
        />
    )
}

const TARGET_STYLE = { minHeight: REPORT_TAP_TARGET }
const NO_PARAMS: Record<string, unknown> = {}
const PICK_PARAM = 'pick'
/** The operators whose value is a record of the linked source. */
const RECORD_OPS: ReadonlySet<string> = new Set(['eq', 'ne', 'in'])

// The spec the record picker reads its options from: the linked source, with the
// one param a run of this condition would ask (askParamFor), so the options are the
// records the reader may list there (R19), shown by the same label field.
function recordPickerSpec(def: ParamDef, target: CatalogSource): ReportSpec {
    const label = def.optionsFrom?.label ?? [{ field: 'id' }]
    return {
        version: CURRENT_SPEC_VERSION,
        source: target.key,
        columns: [{ id: 'label', path: label }],
        params: [def],
    }
}

function chosenRecords(item: ConditionDraft): string[] {
    if (item.op === 'in') return item.values
    return item.value ? [item.value] : []
}

const LOADING_LABEL = 'Loading…'

/**
 * A picked record's shown label: "Loading…" while labels are read (null), the
 * record's label, or (none) for a record the author can no longer list.
 */
function recordLabel(id: string, labels: ChoiceLabels | null | undefined): string {
    if (!labels) return LOADING_LABEL
    return labels.get(id) ?? BLANK_LINK_LABEL
}

function RecordChip({
    label,
    testID,
    onPress,
}: {
    label: string
    testID: string
    onPress: () => void
}) {
    const color = useThemeColor('primary-foreground')
    return (
        <Pressable
            testID={testID}
            accessibilityRole="button"
            accessibilityHint={`Removes ${label}`}
            onPress={onPress}
            className="flex-row items-center gap-1 rounded-full border border-primary bg-primary px-3"
            style={TARGET_STYLE}
        >
            <Text numberOfLines={1} className="text-sm text-primary-foreground">
                {label}
            </Text>
            <X size={14} color={color} />
        </Pressable>
    )
}

function RecordChips({
    isVisible,
    ids,
    labels,
    testID,
    onRemove,
}: {
    isVisible: boolean
    ids: readonly string[]
    labels: ChoiceLabels | null | undefined
    testID: string
    onRemove: (id: string) => void
}) {
    if (!isVisible) return null
    const chips = ids.map(id => (
        <RecordChip
            key={id}
            label={recordLabel(id, labels)}
            testID={`${testID}-${id}`}
            onPress={() => onRemove(id)}
        />
    ))
    return <View className="flex-row flex-wrap gap-1.5">{chips}</View>
}

function triggerText(
    isMultiple: boolean,
    chosen: readonly string[],
    labels: ChoiceLabels | null | undefined,
    noun: string
): string {
    if (isMultiple || chosen.length === 0) return `Choose ${noun}`
    return recordLabel(chosen[0] ?? '', labels)
}

// A record on a link is picked from the linked source's records the reader may list
// (R19): "is" and "is not" pick one; "is any of" shows each picked record as a chip
// that removes it, and picks more from the same searching picker.
function RecordValues({ ctx, item, path, info, target }: ValueProps & { target: CatalogSource }) {
    const testID = `${itemTestId(ctx, path)}-value`
    const isMultiple = item.op === 'in'
    const def = askParamFor(info.field, item.op, PICK_PARAM, target)
    const source = def ? { spec: recordPickerSpec(def, target) } : null
    const chosen = chosenRecords(item)
    const asks = [{ param: PICK_PARAM, values: chosen, groups: [] }]
    const labels = useParamOptionLabels(source, asks, NO_PARAMS).get(PICK_PARAM)
    const isOpen = useReportUIStore(s => s.builderPicker === testID)
    const setPicker = useReportUIStore(s => s.setBuilderPicker)
    if (!def || !source) return null
    const trigger = (
        <Button testID={testID} variant="outline" size="sm" style={TARGET_STYLE}>
            <ButtonText numberOfLines={1}>
                {triggerText(isMultiple, chosen, labels, target.label)}
            </ButtonText>
        </Button>
    )
    const setChosen = (ids: string[]) =>
        setCondition(ctx, path, isMultiple ? { values: ids } : { value: ids[0] ?? '' })
    const onApply = (next: ParamValue) => {
        if (next.kind === 'choice') setChosen(next.values)
        setPicker(null)
    }
    const value: ParamValue | null = chosen.length > 0 ? { kind: 'choice', values: chosen } : null
    return (
        <View className="flex-row flex-wrap items-center gap-1.5">
            <RecordChips
                isVisible={isMultiple}
                ids={chosen}
                labels={labels}
                testID={testID}
                onRemove={id => setChosen(chosen.filter(c => c !== id))}
            />
            <SourcePicker
                trigger={trigger}
                isOpen={isOpen}
                onOpenChange={open => setPicker(open ? testID : null)}
                def={def}
                value={value}
                onApply={onApply}
                source={source}
                params={NO_PARAMS}
                title={target.label}
            />
        </View>
    )
}

const NUMERIC_TYPES = new Set(['number', 'money', 'percent'])

function OneValue({ ctx, item, path, info, part }: ValueProps & { part: 'value' | 'high' }) {
    const testID = `${itemTestId(ctx, path)}-${part}`
    const label = part === 'high' ? `${info.label}, to` : `${info.label} value`
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

// An asked value is set by the reader, and a value that is the key of the group
// above is that key: neither shows a value of its own.
export function ConditionValue(props: ValueProps) {
    const { item, info } = props
    if (!needsValue(item.op) || item.param || item.above) return null
    if (info.field.type === 'relation' && info.target && RECORD_OPS.has(item.op)) {
        return <RecordValues {...props} target={info.target} />
    }
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
            testID={`${itemTestId(ctx, path)}-default`}
        />
    )
}

// Records asked when run can be listed in groups (a team's region), so the reader
// picks a whole group at once with "All <group>" (R30).
function OptionsGroup({ ctx, item, path }: ValueProps) {
    const param = ctx.params.find(p => p.id === item.param)
    const from = param?.optionsFrom
    if (!param || !from) return null
    const testID = `${itemTestId(ctx, path)}-options-group`
    const info = from.group ? describePath(ctx.catalog, from.source, from.group) : null
    const setGroup = (group: SpecHop[] | null) => {
        const next = withOptionsGroup(param, group, ctx.catalog)
        ctx.builder.setParams(ctx.params.map(p => (p.id === param.id ? next : p)))
    }
    return (
        <View className="flex-row items-center gap-2">
            <Text className="text-xs text-muted-foreground">Group the list by</Text>
            <ColumnPicker
                catalog={ctx.catalog}
                baseKey={from.source}
                onPick={setGroup}
                label={info ? pathLabel(info) : 'No groups'}
                testID={testID}
                toOneOnly
                allowLinkEnd
            />
            <IconButton
                label="Stop grouping the list"
                testID={`${testID}-clear`}
                isVisible={from.group !== undefined}
                icon={X}
                onPress={() => setGroup(null)}
            />
        </View>
    )
}

const GROUP_ORDER_LABELS: Record<GroupOrderBy, string> = {
    label: 'Name (A–Z)',
    declared: 'The field’s own order',
    path: 'A field',
}

// A new order by a field starts at the options' own label field: each group then
// comes by its first option.
function orderFor(by: GroupOrderBy, from: OptionsFrom): GroupOrder | undefined {
    if (by === 'declared') return { by }
    if (by === 'path') return { by, path: from.label }
    return undefined
}

function pathOrderLabel(ctx: FilterContext, from: OptionsFrom): string {
    const order = from.groupOrder
    const info = order?.by === 'path' ? describePath(ctx.catalog, from.source, order.path) : null
    return info ? pathLabel(info) : 'Choose a field'
}

// The field a group order by path reads, picked as the group field is.
function GroupOrderField({
    isVisible,
    ctx,
    from,
    testID,
    onPick,
}: {
    isVisible: boolean
    ctx: FilterContext
    from: OptionsFrom
    testID: string
    onPick: (path: SpecHop[]) => void
}) {
    if (!isVisible) return null
    return (
        <ColumnPicker
            catalog={ctx.catalog}
            baseKey={from.source}
            onPick={onPick}
            label={pathOrderLabel(ctx, from)}
            testID={testID}
            toOneOnly
        />
    )
}

// The order of a grouped list's groups (spec.go GroupOrder): by name, in the group
// field's own order (only a select has one), or by the smallest value of a field
// in each group. Name is the default, so it writes no order. The builder's schema
// checks it (draft.ts groupOrderIssues) and the refusal shows below.
function OptionsGroupOrder({ ctx, item, path }: ValueProps) {
    const index = ctx.params.findIndex(p => p.id === item.param)
    const param = ctx.params[index]
    const from = param?.optionsFrom
    if (!param || !from?.group?.length) return null
    const testID = `${itemTestId(ctx, path)}-options-order`
    const current = from.groupOrder?.by ?? 'label'
    const setOrder = (groupOrder: GroupOrder | undefined) => {
        const next = tidyParam({ ...param, optionsFrom: { ...from, groupOrder } })
        ctx.builder.setParams(ctx.params.map(p => (p.id === param.id ? next : p)))
    }
    const items: PickItem[] = groupOrderChoices(ctx.catalog, from).map(by => ({
        id: by,
        label: GROUP_ORDER_LABELS[by],
        isSelected: by === current,
        onSelect: () => {
            if (by !== current) setOrder(orderFor(by, from))
        },
    }))
    const field = `params.${index}.optionsFrom.groupOrder` as const
    const error = ctx.builder.form.getFieldState(field, ctx.builder.form.formState).error
    return (
        <View className="basis-full gap-1">
            <View className="flex-row flex-wrap items-center gap-2">
                <Text className="text-xs text-muted-foreground">Order the groups by</Text>
                <PickMenu
                    label={GROUP_ORDER_LABELS[current]}
                    purpose="Order the groups by"
                    items={items}
                    testID={testID}
                />
                <GroupOrderField
                    isVisible={current === 'path'}
                    ctx={ctx}
                    from={from}
                    testID={`${testID}-field`}
                    onPick={orderPath => setOrder({ by: 'path', path: orderPath })}
                />
            </View>
            <FieldError message={error?.message} testID={`${testID}-error`} />
        </View>
    )
}

// The text of a whole-group pick (D1), as "All {group} teams": a report names its
// groups in its own words. Empty keeps "All <group>". The builder's schema checks
// it (draft.ts paramIssues) and the field shows the refusal.
function GroupPickField({ ctx, item, path }: ValueProps) {
    const index = ctx.params.findIndex(p => p.id === item.param)
    const isGrouped = (ctx.params[index]?.optionsFrom?.group?.length ?? 0) > 0
    if (!isGrouped) return null
    return (
        <View className="basis-full">
            <TextInput
                name={`params.${index}.optionsFrom.groupLabel`}
                control={ctx.builder.form.control}
                label="Whole group text"
                placeholder={`All ${GROUP_LABEL_TOKEN}`}
                hint={`${GROUP_LABEL_TOKEN} is the group’s name`}
                testID={`${itemTestId(ctx, path)}-group-label`}
                maxLength={MAX_LABEL}
                autoCorrect={false}
            />
        </View>
    )
}

// "Ask when run" turns the value into a param the reader sets from a chip (R16); a
// range of numbers or amounts asks both ends (F18). Yes/no cannot be asked. A
// read-back condition that already asks stays switchable off even when its
// operator could not be asked today. A column's asked value takes the column's id.
export function AskToggle(props: ValueProps) {
    const { ctx, item, path, info } = props
    if (item.above) return null
    const isAsking = item.param !== ''
    // A relation is asked as a record picked from the source it points at (R19).
    const canAsk = askParamFor(info.field, item.op, 'probe', info.target) !== null
    if (!isAsking && !canAsk) return null
    const needed = askHighParamFor(info.field, item.op, 'probe') ? 2 : 1
    const isFull = !isAsking && ctx.params.length + needed > MAX_PARAMS
    const onChange = (on: boolean) => {
        if (!on) {
            setCondition(ctx, path, { param: '', paramHigh: '' })
            return
        }
        const taken = ctx.params.map(p => p.id)
        const id = nextColumnId(taken, columnHint(item.path) || item.col)
        const low = askParamFor(info.field, item.op, id, info.target)
        if (!low) return
        const asked = askHighParamFor(info.field, item.op, low.id)
        if (!asked) {
            setCondition(ctx, path, { param: low.id }, low)
            return
        }
        const high = { ...asked, id: nextColumnId([...taken, low.id], asked.id) }
        setCondition(ctx, path, { param: low.id, paramHigh: high.id }, low, high)
    }
    return (
        <View className="flex-row flex-wrap items-center gap-2">
            <Switch
                value={isAsking}
                onValueChange={onChange}
                isDisabled={isFull}
                accessibilityLabel="Ask when the report runs"
                testID={`${itemTestId(ctx, path)}-ask`}
            />
            <Text className="text-sm text-foreground">Ask when the report runs</Text>
            <DefaultPreset {...props} />
            <OptionsGroup {...props} />
            <OptionsGroupOrder {...props} />
            <GroupPickField {...props} />
        </View>
    )
}

const sameValues = (a: readonly string[], b: readonly string[]) =>
    a.length === b.length && a.every(v => b.includes(v))

/**
 * A group above whose key a value can be: a column of the same type, pointing at
 * the same source (a relation) or holding the same options (a select).
 */
function matchesAbove(group: AboveChoice, info: ConditionInfo): boolean {
    const { type, options } = info.field
    if (group.type !== type) return false
    if (type === 'relation') return group.target !== undefined && group.target === info.target?.key
    if (type === 'select') {
        const values = (options ?? []).map(o => o.value)
        return group.options !== undefined && sameValues(group.options, values)
    }
    return true
}

// In a fill under other groups, a value can be the key of a group above, so each
// parent shows its own records (a team's region is the region above, F21).
export function AboveMenu({ ctx, item, path, info }: ValueProps) {
    const offered = ctx.above.filter(g => matchesAbove(g, info) || g.id === item.above)
    if (!needsValue(item.op) || item.op === 'in' || offered.length === 0) return null
    const clear = { param: '', paramHigh: '', value: '', high: '' }
    const items: PickItem[] = [
        {
            id: ':value',
            label: 'A value',
            isSelected: item.above === '',
            onSelect: () => setCondition(ctx, path, { above: '' }),
        },
        ...offered.map(g => ({
            id: g.id,
            label: `Same as the ${g.label} above`,
            isSelected: g.id === item.above,
            onSelect: () => setCondition(ctx, path, { above: g.id, ...clear }),
        })),
    ]
    const shown = items.find(i => i.isSelected)?.label ?? `${item.above} (missing)`
    return (
        <PickMenu
            label={shown}
            purpose={`${info.label} compared with`}
            items={items}
            testID={`${itemTestId(ctx, path)}-above`}
        />
    )
}

// An average's decimal places: whole cents for amounts (formula.go checkAggregate),
// else the author's, starting at the number default.
export function TotalPlaces({ ctx, item, path, info }: ValueProps) {
    if (item.subject !== 'total' || item.aggregate !== 'avg') return null
    const id = itemTestId(ctx, path)
    if (info.field.type === 'money') {
        return (
            <Text className="text-xs text-muted-foreground" testID={`${id}-cents`}>
                Whole cents
            </Text>
        )
    }
    return (
        <FormulaPlacesPick
            places={item.scale ?? avgScaleFor(info.field.type)}
            onChange={scale => setCondition(ctx, path, { scale })}
            purpose={`${info.label} decimal places`}
            testID={`${id}-places`}
        />
    )
}
