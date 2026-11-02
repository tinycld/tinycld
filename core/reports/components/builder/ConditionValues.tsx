import { FormulaPlacesPick } from '@tinycld/core/components/forms-builder/FormulaEditor'
import { DateField } from '@tinycld/core/ui/forms/fields/DateField'
import { PlainInput } from '@tinycld/core/ui/PlainInput'
import { type PickItem, PickMenu } from '@tinycld/core/ui/pick-menu'
import { Switch } from '@tinycld/core/ui/switch'
import { Pressable, Text, View } from 'react-native'
import { avgScaleFor, columnHint, MAX_PARAMS, nextColumnId } from '../../lib/builder/columns'
import {
    askHighParamFor,
    askParamFor,
    type ConditionDraft,
    type FilterPath,
    needsValue,
} from '../../lib/builder/filter'
import { isDateParam, PRESET_LABELS, presetsFor } from '../../lib/params'
import type { ParamDef } from '../../lib/spec'
import { REPORT_TAP_TARGET } from '../ReportHeader'
import {
    type AboveChoice,
    type ConditionInfo,
    type FilterContext,
    itemTestId,
    setCondition,
} from './condition-context'

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
