import { NumberInput, TextInput, useForm, useWatch, zodResolver } from '@tinycld/core/ui/form'
import { Popover } from '@tinycld/core/ui/popover'
import type { ReactElement } from 'react'
import { ScrollView, View } from 'react-native'
import type { OptionsSource } from '../hooks/useParamOptions'
import { textFromCents } from '../lib/money'
import {
    type AmountValue,
    amountParamValue,
    amountValueSchema,
    choicesSchema,
    type NumberValue,
    numberValueSchema,
    textValueSchema,
} from '../lib/param-forms'
import { ApplyButton, type BodyProps, LIST_STYLE, OptionRow } from './ParamPickerParts'
import { SourceChoiceBody } from './SourceChoicePicker'

const PICKER_WIDTH = 340

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
    const form = useForm<NumberValue>({
        resolver: zodResolver(numberValueSchema),
        mode: 'onChange',
        // Empty until a number is typed, so Apply never applies a number nobody chose.
        defaultValues: { number: value?.kind === 'number' ? value.number : undefined },
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

// An amount in a currency (R29): a blank currency compares with an amount in any.
function AmountBody({ def, value, onApply }: BodyProps) {
    const current = value?.kind === 'amount' ? value : null
    const form = useForm<AmountValue>({
        resolver: zodResolver(amountValueSchema),
        mode: 'onChange',
        defaultValues: {
            amount: current ? textFromCents(current.cents) : '',
            currency: current?.currency ?? '',
        },
    })
    const apply = () => void form.handleSubmit(submitted => onApply(amountParamValue(submitted)))()
    return (
        <View className="gap-2">
            <TextInput
                name="amount"
                control={form.control}
                label={def.label ?? 'Amount'}
                keyboardType="decimal-pad"
                testID="report-filter-value"
                onSubmitEditing={apply}
            />
            <TextInput
                name="currency"
                control={form.control}
                label="Currency"
                autoCapitalize="characters"
                placeholder="Any currency"
                maxLength={3}
                testID="report-filter-currency"
                onSubmitEditing={apply}
            />
            <ApplyButton isDisabled={!form.formState.isValid} onPress={apply} />
        </View>
    )
}

interface PickerBodyProps extends BodyProps {
    isVisible: boolean
    source: OptionsSource | null
    params: Record<string, unknown>
}

function PickerBody({ isVisible, source, params, ...props }: PickerBodyProps) {
    if (!isVisible) return null
    const { def } = props
    if (def.type === 'choice' && def.optionsFrom && source) {
        return <SourceChoiceBody {...props} source={source} params={params} />
    }
    if (def.type === 'choice' && def.multiple) return <MultiChoiceBody {...props} />
    if (def.type === 'choice') return <SingleChoiceBody {...props} />
    if (def.type === 'amount') return <AmountBody {...props} />
    if (def.type === 'number') return <NumberBody {...props} />
    return <TextBody {...props} />
}

interface ParamPickerProps extends BodyProps {
    trigger: ReactElement
    isOpen: boolean
    onOpenChange(open: boolean): void
    /** Where a choice from a source reads its options; null while the report loads. */
    source: OptionsSource | null
    /** The run's other params, which an options filter may read. */
    params: Record<string, unknown>
    title?: string
}

// The picker of a choice, text, number or amount param. Closing without Apply
// changes nothing. On a phone it is a bottom sheet.
export function ParamPicker({ trigger, isOpen, onOpenChange, title, ...body }: ParamPickerProps) {
    return (
        <Popover
            trigger={trigger}
            isOpen={isOpen}
            onOpenChange={onOpenChange}
            title={title ?? body.def.label ?? 'Filter'}
            sheetSide="bottom"
            width={PICKER_WIDTH}
            testID="report-filter-picker"
        >
            <PickerBody isVisible={isOpen} {...body} />
        </Popover>
    )
}

/** The searching record picker over a source, for the builder's record values (R19). */
export function SourcePicker(props: ParamPickerProps & { source: OptionsSource }) {
    return <ParamPicker {...props} />
}
