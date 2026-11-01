import { useForm, useWatch, zodResolver } from '@tinycld/core/ui/form'
import { DateField } from '@tinycld/core/ui/forms/fields/DateField'
import { Popover } from '@tinycld/core/ui/popover'
import type { ReactElement } from 'react'
import { Pressable, Text, View } from 'react-native'
import {
    CUSTOM,
    type PeriodValues,
    periodDefaults,
    periodSchema,
    periodValueOf,
} from '../lib/param-forms'
import { CUSTOM_LABEL, type ParamValue, PRESET_LABELS, presetsFor } from '../lib/params'
import type { ParamDef } from '../lib/spec'
import type { ResolvedParam } from '../lib/types'
import { REPORT_TAP_TARGET } from './ReportHeader'

const POPOVER_WIDTH = 360
const PILL_STYLE = { minHeight: REPORT_TAP_TARGET }
const PILL_TONE = {
    selected: { box: 'border-primary bg-primary', text: 'text-primary-foreground' },
    idle: { box: 'border-border bg-transparent', text: 'text-foreground' },
}

interface PresetPillProps {
    id: string
    label: string
    isSelected: boolean
    onPress: () => void
}

function PresetPill({ id, label, isSelected, onPress }: PresetPillProps) {
    const tone = isSelected ? PILL_TONE.selected : PILL_TONE.idle
    return (
        <Pressable
            testID={`report-period-preset-${id}`}
            accessibilityRole="radio"
            accessibilityState={{ checked: isSelected }}
            aria-checked={isSelected}
            onPress={onPress}
            className={`justify-center rounded-full border px-3 ${tone.box}`}
            style={PILL_STYLE}
        >
            <Text className={`text-sm font-medium ${tone.text}`}>{label}</Text>
        </Pressable>
    )
}

function FieldError({ message }: { message: string | undefined }) {
    if (!message) return null
    return (
        <Text testID="report-period-error" role="alert" className="text-xs text-danger">
            {message}
        </Text>
    )
}

interface PeriodDateProps {
    isVisible: boolean
    label: string
    value: string | null
    error: string | undefined
    testID: string
    onChange: (value: string | null) => void
}

function PeriodDate({ isVisible, label, value, error, testID, onChange }: PeriodDateProps) {
    if (!isVisible) return null
    return (
        <View className="gap-1.5">
            <Text className="text-sm font-semibold text-foreground">{label}</Text>
            <DateField
                value={value}
                onChange={onChange}
                withTime={false}
                accessibilityLabel={label}
                testID={testID}
            />
            <FieldError message={error} />
        </View>
    )
}

interface BodyProps {
    def: ParamDef
    value: ParamValue | null
    resolved: ResolvedParam | undefined
    onApply(value: ParamValue, keepOpen: boolean): void
}

// The form lives while the popover is open, so each open starts from the value shown,
// and closing with a bad typed date discards it.
function OpenPeriodBody({ def, value, resolved, onApply }: BodyProps) {
    const form = useForm<PeriodValues>({
        resolver: zodResolver(periodSchema(def)),
        mode: 'onChange',
        defaultValues: periodDefaults(value, resolved),
    })
    const [preset, from, to] = useWatch({ control: form.control, name: ['preset', 'from', 'to'] })
    const errors = form.formState.errors
    const isRange = def.type === 'dateRange'
    const apply = (keepOpen: boolean) =>
        void form.handleSubmit(values => {
            const next = periodValueOf(def, values)
            if (next) onApply(next, keepOpen)
        })()
    const onPreset = (id: string) => {
        form.setValue('preset', id)
        if (id !== CUSTOM) apply(false)
    }
    // A typed day makes the period Custom and applies once the period is valid,
    // keeping the popover open so a date error stays in view.
    const onDate = (field: 'from' | 'to', next: string | null) => {
        form.setValue('preset', CUSTOM)
        form.setValue(field, next)
        apply(true)
    }
    const presetPills = presetsFor(def).map(id => (
        <PresetPill
            key={id}
            id={id}
            label={PRESET_LABELS[id]}
            isSelected={id === preset}
            onPress={() => onPreset(id)}
        />
    ))
    const toLabel = isRange ? 'To' : 'As of'
    const toTestID = isRange ? 'report-period-to' : 'report-period-as-of'
    return (
        <View className="gap-3 p-2">
            <View accessibilityRole="radiogroup" className="flex-row flex-wrap gap-2">
                {presetPills}
                <PresetPill
                    id={CUSTOM}
                    label={CUSTOM_LABEL}
                    isSelected={preset === CUSTOM}
                    onPress={() => onPreset(CUSTOM)}
                />
            </View>
            <PeriodDate
                isVisible={isRange}
                label="From"
                value={from}
                error={errors.from?.message}
                testID="report-period-from"
                onChange={next => onDate('from', next)}
            />
            <PeriodDate
                isVisible
                label={toLabel}
                value={to}
                error={errors.to?.message}
                testID={toTestID}
                onChange={next => onDate('to', next)}
            />
        </View>
    )
}

function PeriodBody({ isVisible, ...props }: BodyProps & { isVisible: boolean }) {
    if (!isVisible) return null
    return <OpenPeriodBody {...props} />
}

interface ParamPeriodControlProps extends BodyProps {
    trigger: ReactElement
    isOpen: boolean
    onOpenChange(open: boolean): void
}

/** The period control of a date param: presets that apply at once, or Custom days. */
export function ParamPeriodControl({
    trigger,
    isOpen,
    onOpenChange,
    ...body
}: ParamPeriodControlProps) {
    return (
        <Popover
            trigger={trigger}
            isOpen={isOpen}
            onOpenChange={onOpenChange}
            title={body.def.label ?? 'Report period'}
            sheetSide="bottom"
            width={POPOVER_WIDTH}
            testID="report-period-popover"
        >
            <PeriodBody isVisible={isOpen} {...body} />
        </Popover>
    )
}
