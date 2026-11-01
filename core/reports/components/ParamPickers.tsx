import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { NumberInput, TextInput, useForm, useWatch, zodResolver } from '@tinycld/core/ui/form'
import { Popover } from '@tinycld/core/ui/popover'
import { Check } from 'lucide-react-native'
import type { ReactElement } from 'react'
import { Pressable, ScrollView, Text, View } from 'react-native'
import { choicesSchema, numberValueSchema, textValueSchema } from '../lib/param-forms'
import type { ParamValue } from '../lib/params'
import type { ParamDef } from '../lib/spec'
import { REPORT_TAP_TARGET } from './ReportHeader'

const PICKER_WIDTH = 340
const LIST_STYLE = { maxHeight: 360 }
const TARGET_STYLE = { minHeight: REPORT_TAP_TARGET }

function ApplyButton({ isDisabled, onPress }: { isDisabled: boolean; onPress: () => void }) {
    return (
        <View className="border-t border-border pt-2">
            <Button
                testID="report-filter-apply"
                style={TARGET_STYLE}
                isDisabled={isDisabled}
                onPress={onPress}
            >
                <ButtonText>Apply</ButtonText>
            </Button>
        </View>
    )
}

function CheckMark({ isVisible }: { isVisible: boolean }) {
    const color = useThemeColor('primary-foreground')
    if (!isVisible) return null
    return <Check size={14} color={color} />
}

interface OptionRowProps {
    value: string
    label: string
    isChecked: boolean
    isRadio: boolean
    onPress: () => void
}

function OptionRow({ value, label, isChecked, isRadio, onPress }: OptionRowProps) {
    const tone = isChecked ? 'border-primary bg-primary' : 'border-border bg-transparent'
    const shape = isRadio ? 'rounded-full' : 'rounded'
    const role = isRadio ? 'radio' : 'checkbox'
    return (
        <Pressable
            testID={`report-filter-option-${value}`}
            accessibilityRole={role}
            accessibilityState={{ checked: isChecked }}
            aria-checked={isChecked}
            onPress={onPress}
            className="flex-row items-center gap-3 rounded-md px-2"
            style={TARGET_STYLE}
        >
            <View className={`h-5 w-5 items-center justify-center border ${shape} ${tone}`}>
                <CheckMark isVisible={isChecked} />
            </View>
            <Text numberOfLines={1} className="min-w-0 flex-1 text-sm text-foreground">
                {label}
            </Text>
        </Pressable>
    )
}

interface BodyProps {
    def: ParamDef
    value: ParamValue | null
    onApply(value: ParamValue): void
}

// One choice applies when it is pressed.
function SingleChoiceBody({ def, value, onApply }: BodyProps) {
    const current = value?.kind === 'choice' ? value.values[0] : undefined
    const rows = (def.options ?? []).map(option => (
        <OptionRow
            key={option.value}
            value={option.value}
            label={option.label}
            isChecked={option.value === current}
            isRadio
            onPress={() => onApply({ kind: 'choice', values: [option.value] })}
        />
    ))
    return (
        <ScrollView style={LIST_STYLE} nestedScrollEnabled>
            {rows}
        </ScrollView>
    )
}

// Several choices apply on Apply; an empty selection is not a filter.
function MultiChoiceBody({ def, value, onApply }: BodyProps) {
    const form = useForm({
        resolver: zodResolver(choicesSchema),
        mode: 'onChange',
        defaultValues: { values: value?.kind === 'choice' ? value.values : [] },
    })
    const values = useWatch({ control: form.control, name: 'values' })
    const toggle = (option: string) => {
        const next = values.includes(option)
            ? values.filter(v => v !== option)
            : [...values, option]
        form.setValue('values', next, { shouldValidate: true })
    }
    const rows = (def.options ?? []).map(option => (
        <OptionRow
            key={option.value}
            value={option.value}
            label={option.label}
            isChecked={values.includes(option.value)}
            isRadio={false}
            onPress={() => toggle(option.value)}
        />
    ))
    const apply = () =>
        void form.handleSubmit(submitted => onApply({ kind: 'choice', values: submitted.values }))()
    return (
        <View className="gap-2">
            <ScrollView style={LIST_STYLE} nestedScrollEnabled>
                {rows}
            </ScrollView>
            <ApplyButton isDisabled={!form.formState.isValid} onPress={apply} />
        </View>
    )
}

function TextBody({ def, value, onApply }: BodyProps) {
    const form = useForm({
        resolver: zodResolver(textValueSchema),
        mode: 'onChange',
        defaultValues: { text: value?.kind === 'text' ? value.text : '' },
    })
    const apply = () =>
        void form.handleSubmit(submitted => onApply({ kind: 'text', text: submitted.text }))()
    return (
        <View className="gap-2">
            <TextInput
                name="text"
                control={form.control}
                label={def.label ?? 'Value'}
                testID="report-filter-value"
                onSubmitEditing={apply}
            />
            <ApplyButton isDisabled={!form.formState.isValid} onPress={apply} />
        </View>
    )
}

function NumberBody({ def, value, onApply }: BodyProps) {
    const form = useForm({
        resolver: zodResolver(numberValueSchema),
        mode: 'onChange',
        defaultValues: { number: value?.kind === 'number' ? value.number : 0 },
    })
    const apply = () =>
        void form.handleSubmit(submitted => onApply({ kind: 'number', number: submitted.number }))()
    return (
        <View className="gap-2">
            <NumberInput
                name="number"
                control={form.control}
                label={def.label ?? 'Value'}
                allowDecimal
                testID="report-filter-value"
            />
            <ApplyButton isDisabled={!form.formState.isValid} onPress={apply} />
        </View>
    )
}

function PickerBody({ isVisible, ...props }: BodyProps & { isVisible: boolean }) {
    if (!isVisible) return null
    if (props.def.type === 'choice' && props.def.multiple) return <MultiChoiceBody {...props} />
    if (props.def.type === 'choice') return <SingleChoiceBody {...props} />
    if (props.def.type === 'number') return <NumberBody {...props} />
    return <TextBody {...props} />
}

interface ParamPickerProps extends BodyProps {
    trigger: ReactElement
    isOpen: boolean
    onOpenChange(open: boolean): void
}

// The picker of a choice, text or number param. Closing without Apply changes nothing.
export function ParamPicker({ trigger, isOpen, onOpenChange, ...body }: ParamPickerProps) {
    return (
        <Popover
            trigger={trigger}
            isOpen={isOpen}
            onOpenChange={onOpenChange}
            title={body.def.label ?? 'Filter'}
            sheetSide="bottom"
            width={PICKER_WIDTH}
            testID="report-filter-picker"
        >
            <PickerBody isVisible={isOpen} {...body} />
        </Popover>
    )
}
