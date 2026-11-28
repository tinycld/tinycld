import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useCoveredInputProps } from '@tinycld/core/ui/overlay/covered'
import { Minus, Plus } from 'lucide-react-native'
import { type ReactNode, type Ref, useState } from 'react'
import { type Control, type FieldValues, type Path, useController } from 'react-hook-form'
import {
    Platform,
    Pressable,
    TextInput as RNTextInput,
    Text,
    type TextInputFocusEvent,
    type TextInputInstance,
    View,
    type ViewProps,
} from 'react-native'
import { iosInputCenteringStyle } from './ios-input-style'
import { numberKeyboardType, parseNumberText } from './number-input-text'

type NumberInputCommonProps = {
    label?: string
    hint?: string
    /** The caller renders the error itself (a form cell does), so the input only colours its border. */
    hideError?: boolean
    increment?: number
    min?: number
    max?: number
    disabled?: boolean
    /** Accepts fractions. Off by default, so an integer field stays integer. */
    allowDecimal?: boolean
    /** Shown while empty: say what empty means ("No limit"). */
    placeholder?: string
    /** A narrow stepper that sits at the start of its row instead of filling it. */
    isCompact?: boolean
    /**
     * No − and + buttons, so the input is the field's only Tab stop (a row of
     * inputs the keyboard walks through). The input then fills its wrapper;
     * size it with `wrapperProps`.
     */
    hideSteppers?: boolean
    testID?: string
    /** Names the input for assistive technology when no visible label is passed. */
    accessibilityLabel?: string
    wrapperProps?: ViewProps
    /** Enter on web, the return key on native (a table adds a row on it). */
    onSubmitEditing?: () => void
    /** Enter (the return key on native) keeps the focus in the input instead of leaving it. */
    keepFocusOnSubmit?: boolean
    /** Selects the whole text when the input takes focus, so a typed number replaces it. */
    selectTextOnFocus?: boolean
    /** `end`: right aligned with tabular figures, for a column of numbers. Centered by default. */
    align?: 'center' | 'end'
    /** The input element, for a caller that moves focus to it (a table after a row action). */
    inputRef?: Ref<TextInputInstance>
    /** Called with the focus event when the input takes focus (a table notes which row the keyboard is in). */
    onFocus?: (event: TextInputFocusEvent) => void
    /** Called when the input loses focus (a table forgets the row the keyboard left). */
    onBlur?: () => void
}

/** Bound to a react-hook-form field: the field holds the number (or the partial text). */
type NumberInputFieldProps<T extends FieldValues> = {
    name: Path<T>
    control: Control<T>
    value?: never
    onChangeText?: never
    error?: never
}

/**
 * Text mode, for a caller that keeps the text itself (a table cell whose text
 * drives other cells): no form field. `onChangeText` gets each accepted text,
 * with U+2212 read as a minus; `error` colours the border and shows under the
 * input unless `hideError`.
 */
type NumberInputTextProps = {
    value: string
    onChangeText: (text: string) => void
    error?: string
    name?: never
    control?: never
}

/** The props of a NumberInput bound to a form field (the mode every caller had before text mode). */
export type NumberInputFieldModeProps<T extends FieldValues = Record<string, unknown>> =
    NumberInputCommonProps & NumberInputFieldProps<T>

export type NumberInputProps<T extends FieldValues = Record<string, unknown>> =
    | NumberInputFieldModeProps<T>
    | (NumberInputCommonProps & NumberInputTextProps)

function decimalsOf(value: number): number {
    return String(value).split('.')[1]?.length ?? 0
}

/** Adds without float noise: 0.1 + 0.2 stays 0.3, at the finer of the two precisions. */
function addPrecisely(current: number, delta: number): number {
    return Number((current + delta).toFixed(Math.max(decimalsOf(current), decimalsOf(delta))))
}

interface StepperProps {
    isVisible: boolean
    onPress: () => void
    isEnabled: boolean
    label: string
    sizeClass: string
    children: ReactNode
}

function Stepper({ isVisible, onPress, isEnabled, label, sizeClass, children }: StepperProps) {
    if (!isVisible) return null
    return (
        <Pressable
            onPress={onPress}
            disabled={!isEnabled}
            accessibilityRole="button"
            accessibilityLabel={label}
            className={`shrink-0 items-center justify-center rounded-full border border-border bg-background ${sizeClass} ${isEnabled ? 'opacity-100' : 'opacity-40'}`}
        >
            {children}
        </Pressable>
    )
}

function isTextMode<T extends FieldValues>(
    props: NumberInputProps<T>
): props is NumberInputCommonProps & NumberInputTextProps {
    return props.onChangeText !== undefined
}

export function NumberInput<T extends FieldValues = Record<string, unknown>>(
    props: NumberInputProps<T>
) {
    if (isTextMode(props)) return <TextNumberInput {...props} />
    return <FieldNumberInput {...props} />
}

function stepped(current: number, delta: number, allowDecimal: boolean): number {
    return allowDecimal ? addPrecisely(current, delta) : current + delta
}

function FieldNumberInput<T extends FieldValues>(
    props: NumberInputCommonProps & NumberInputFieldProps<T>
) {
    const { name, control, min, max, disabled = false } = props
    const allowDecimal = props.allowDecimal ?? false
    const {
        field,
        fieldState: { error },
    } = useController({ name, control })

    // What the user is typing, kept while it is not yet the canonical text of a number ("1.", "0.50").
    const [typed, setTyped] = useState<string | null>(null)

    const handleTextChange = (raw: string) => {
        const parsed = parseNumberText(raw, { allowDecimal, min, max })
        if (!parsed) return
        if (allowDecimal) setTyped(parsed.text)
        field.onChange(parsed.value ?? parsed.text)
    }

    const handleStep = (delta: number) => {
        const current = typeof field.value === 'number' ? field.value : 0
        const next = stepped(current, delta, allowDecimal)
        setTyped(null)
        if (delta > 0 && max !== undefined && next > max) return
        if (delta < 0 && min !== undefined && next < min) return
        field.onChange(next)
    }

    const handleBlur = () => {
        setTyped(null)
        field.onBlur()
        props.onBlur?.()
    }

    const isTypedCurrent =
        typed !== null && (typeof field.value !== 'number' || Number(typed) === field.value)
    const displayValue = isTypedCurrent
        ? typed
        : field.value === '' || field.value === '-'
          ? field.value
          : String(field.value ?? '')
    const canDecrement = !disabled && (min === undefined || field.value > min)
    const canIncrement = !disabled && (max === undefined || field.value < max)
    const stepName = props.accessibilityLabel ?? props.label ?? name
    const inputTestID = props.testID ?? name

    return (
        <NumberInputView
            {...props}
            displayValue={displayValue}
            onTextChange={handleTextChange}
            onStep={handleStep}
            onInputBlur={handleBlur}
            canDecrement={canDecrement}
            canIncrement={canIncrement}
            errorMessage={error?.message}
            hasError={!!error}
            stepName={stepName}
            inputTestID={inputTestID}
        />
    )
}

function TextNumberInput(props: NumberInputCommonProps & NumberInputTextProps) {
    const { value, onChangeText, min, max, disabled = false } = props
    const allowDecimal = props.allowDecimal ?? false
    const current = Number(value)
    const currentNumber = Number.isFinite(current) ? current : 0

    const handleTextChange = (raw: string) => {
        const parsed = parseNumberText(raw, { allowDecimal, min, max })
        if (parsed) onChangeText(parsed.text)
    }

    const handleStep = (delta: number) => {
        const next = stepped(currentNumber, delta, allowDecimal)
        if (delta > 0 && max !== undefined && next > max) return
        if (delta < 0 && min !== undefined && next < min) return
        onChangeText(String(next))
    }

    const handleBlur = () => props.onBlur?.()
    const canDecrement = !disabled && (min === undefined || currentNumber > min)
    const canIncrement = !disabled && (max === undefined || currentNumber < max)
    const stepName = props.accessibilityLabel ?? props.label ?? 'value'
    const hasError = !!props.error

    return (
        <NumberInputView
            {...props}
            displayValue={value}
            onTextChange={handleTextChange}
            onStep={handleStep}
            onInputBlur={handleBlur}
            canDecrement={canDecrement}
            canIncrement={canIncrement}
            errorMessage={props.error}
            hasError={hasError}
            stepName={stepName}
            inputTestID={props.testID}
        />
    )
}

type NumberInputViewProps = NumberInputCommonProps & {
    displayValue: string
    onTextChange: (text: string) => void
    onStep: (delta: number) => void
    onInputBlur: () => void
    canDecrement: boolean
    canIncrement: boolean
    hasError: boolean
    errorMessage: string | undefined
    stepName: string
    inputTestID: string | undefined
}

function NumberInputView({
    label,
    hint,
    hideError = false,
    increment = 1,
    min,
    allowDecimal = false,
    disabled = false,
    placeholder,
    isCompact = false,
    hideSteppers = false,
    accessibilityLabel,
    wrapperProps = {},
    onSubmitEditing,
    keepFocusOnSubmit = false,
    selectTextOnFocus,
    align = 'center',
    inputRef,
    onFocus,
    displayValue,
    onTextChange,
    onStep,
    onInputBlur,
    canDecrement,
    canIncrement,
    hasError,
    errorMessage,
    stepName,
    inputTestID,
}: NumberInputViewProps) {
    const placeholderColor = useThemeColor('field-placeholder')
    const iconColor = useThemeColor('foreground')
    const { editable } = useCoveredInputProps({ editable: !disabled })

    const stepSize = isCompact ? 'size-8' : 'size-11'
    const rowClass = hideSteppers
        ? 'flex-row items-center'
        : `flex-row items-center ${isCompact ? 'self-start gap-1.5' : 'gap-2'}`
    // min-w-0: on web the <input> otherwise keeps its intrinsic width and
    // pushes the + stepper into the next field (or out of its wrapper).
    const inputWidthClass = hideSteppers
        ? `flex-1 min-w-0 ${isCompact ? 'px-2' : 'px-3'}`
        : isCompact
          ? 'w-20 px-2'
          : 'flex-1 min-w-0 px-3'
    const alignClass = align === 'end' ? 'text-right tabular-nums' : 'text-center'
    // Native reads submitBehavior; react-native-web reads only blurOnSubmit.
    const submitBehavior = keepFocusOnSubmit ? 'submit' : undefined
    const blurOnSubmit = keepFocusOnSubmit ? false : undefined
    const showHint = !!hint && !hasError
    const showError = hasError && !hideError
    const onDecrement = () => onStep(-increment)
    const onIncrement = () => onStep(increment)
    const keyboardType = numberKeyboardType({ allowDecimal, min }, Platform.OS)

    return (
        <View className="gap-1.5 mb-3" {...wrapperProps}>
            {label ? <Text className="text-sm font-semibold text-foreground">{label}</Text> : null}
            <View className={rowClass}>
                <Stepper
                    isVisible={!hideSteppers}
                    onPress={onDecrement}
                    isEnabled={canDecrement}
                    label={`Decrease ${stepName}`}
                    sizeClass={stepSize}
                >
                    <Minus size={16} color={iconColor} />
                </Stepper>
                <RNTextInput
                    ref={inputRef}
                    value={displayValue}
                    onChangeText={onTextChange}
                    onFocus={onFocus}
                    onBlur={onInputBlur}
                    keyboardType={keyboardType}
                    onSubmitEditing={onSubmitEditing}
                    submitBehavior={submitBehavior}
                    blurOnSubmit={blurOnSubmit}
                    selectTextOnFocus={selectTextOnFocus}
                    editable={editable}
                    accessibilityLabel={accessibilityLabel ?? label}
                    testID={inputTestID}
                    placeholder={placeholder}
                    placeholderTextColor={placeholderColor}
                    className={`${inputWidthClass} border rounded-lg py-2.5 ${alignClass} text-base text-foreground bg-background ${hasError ? 'border-danger' : 'border-border'}`}
                    style={iosInputCenteringStyle(16)}
                />
                <Stepper
                    isVisible={!hideSteppers}
                    onPress={onIncrement}
                    isEnabled={canIncrement}
                    label={`Increase ${stepName}`}
                    sizeClass={stepSize}
                >
                    <Plus size={16} color={iconColor} />
                </Stepper>
            </View>
            <HintText isVisible={showHint} text={hint} />
            <ErrorText isVisible={showError} text={errorMessage} />
        </View>
    )
}

function HintText({ isVisible, text }: { isVisible: boolean; text: string | undefined }) {
    if (!isVisible) return null
    return <Text className="text-xs text-muted">{text}</Text>
}

function ErrorText({ isVisible, text }: { isVisible: boolean; text: string | undefined }) {
    if (!isVisible) return null
    return <Text className="text-xs text-danger">{text}</Text>
}
