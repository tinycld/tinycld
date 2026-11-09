import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Minus, Plus } from 'lucide-react-native'
import { type Ref, useState } from 'react'
import { type Control, type FieldValues, type Path, useController } from 'react-hook-form'
import {
    Pressable,
    TextInput as RNTextInput,
    Text,
    type TextInputInstance,
    View,
    type ViewProps,
} from 'react-native'
import { iosInputCenteringStyle } from './ios-input-style'

export type NumberInputProps<T extends FieldValues = Record<string, unknown>> = {
    name: Path<T>
    control: Control<T>
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
    testID?: string
    /** Names the input for assistive technology when no visible label is passed. */
    accessibilityLabel?: string
    wrapperProps?: ViewProps
    /** Enter on web, the return key on native (a table adds a row on it). */
    onSubmitEditing?: () => void
    /** The input element, for a caller that moves focus to it (a table after a row action). */
    inputRef?: Ref<TextInputInstance>
    /** Called when the input takes focus (a table notes which row the keyboard is in). */
    onFocus?: () => void
    /** Called when the input loses focus (a table forgets the row the keyboard left). */
    onBlur?: () => void
}

const DECIMAL_TEXT = /^-?\d*\.?\d*$/

function decimalsOf(value: number): number {
    return String(value).split('.')[1]?.length ?? 0
}

/** Adds without float noise: 0.1 + 0.2 stays 0.3, at the finer of the two precisions. */
function addPrecisely(current: number, delta: number): number {
    return Number((current + delta).toFixed(Math.max(decimalsOf(current), decimalsOf(delta))))
}

export function NumberInput<T extends FieldValues = Record<string, unknown>>({
    name,
    control,
    label,
    hint,
    hideError = false,
    increment = 1,
    min,
    max,
    disabled = false,
    allowDecimal = false,
    placeholder,
    isCompact = false,
    testID,
    accessibilityLabel,
    wrapperProps = {},
    onSubmitEditing,
    inputRef,
    onFocus,
    onBlur,
}: NumberInputProps<T>) {
    const placeholderColor = useThemeColor('field-placeholder')
    const iconColor = useThemeColor('foreground')
    const {
        field,
        fieldState: { error },
    } = useController({ name, control })

    // What the user is typing, kept while it is not yet the canonical text of a number ("1.", "0.50").
    const [typed, setTyped] = useState<string | null>(null)

    const hasError = !!error

    const handleDecimalChange = (text: string) => {
        if (!DECIMAL_TEXT.test(text)) return
        const numValue = Number(text)
        setTyped(text)
        if (text === '' || !Number.isFinite(numValue)) {
            field.onChange(text)
            return
        }
        if (min !== undefined && numValue < min) return
        if (max !== undefined && numValue > max) return
        field.onChange(numValue)
    }

    const handleTextChange = (text: string) => {
        if (allowDecimal) {
            handleDecimalChange(text)
            return
        }
        if (text === '' || text === '-') {
            field.onChange(text)
            return
        }
        const digitsOnly = text.replace(/[^0-9-]/g, '')
        if (digitsOnly !== text) return

        const numValue = Number.parseInt(digitsOnly, 10)
        if (Number.isNaN(numValue)) return
        if (min !== undefined && numValue < min) return
        if (max !== undefined && numValue > max) return
        field.onChange(numValue)
    }

    const handleIncrement = () => {
        const current = typeof field.value === 'number' ? field.value : 0
        const next = allowDecimal ? addPrecisely(current, increment) : current + increment
        setTyped(null)
        if (max === undefined || next <= max) field.onChange(next)
    }

    const handleDecrement = () => {
        const current = typeof field.value === 'number' ? field.value : 0
        const next = allowDecimal ? addPrecisely(current, -increment) : current - increment
        setTyped(null)
        if (min === undefined || next >= min) field.onChange(next)
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
    const stepSize = isCompact ? 'size-8' : 'size-11'
    const stepName = accessibilityLabel ?? label ?? name

    return (
        <View className="gap-1.5 mb-3" {...wrapperProps}>
            {label ? <Text className="text-sm font-semibold text-foreground">{label}</Text> : null}
            <View className={`flex-row items-center ${isCompact ? 'self-start gap-1.5' : 'gap-2'}`}>
                <Pressable
                    onPress={handleDecrement}
                    disabled={!canDecrement}
                    accessibilityRole="button"
                    accessibilityLabel={`Decrease ${stepName}`}
                    className={`items-center justify-center rounded-full border border-border bg-background ${stepSize} ${canDecrement ? 'opacity-100' : 'opacity-40'}`}
                >
                    <Minus size={16} color={iconColor} />
                </Pressable>
                <RNTextInput
                    ref={inputRef}
                    value={displayValue}
                    onChangeText={handleTextChange}
                    onFocus={onFocus}
                    onBlur={() => {
                        setTyped(null)
                        field.onBlur()
                        onBlur?.()
                    }}
                    keyboardType="numeric"
                    onSubmitEditing={onSubmitEditing}
                    editable={!disabled}
                    accessibilityLabel={accessibilityLabel ?? label}
                    testID={testID ?? name}
                    placeholder={placeholder}
                    placeholderTextColor={placeholderColor}
                    className={`${isCompact ? 'w-20 px-2' : 'flex-1 px-3'} border rounded-lg py-2.5 text-center text-base text-foreground bg-background ${hasError ? 'border-danger' : 'border-border'}`}
                    style={iosInputCenteringStyle(16)}
                />
                <Pressable
                    onPress={handleIncrement}
                    disabled={!canIncrement}
                    accessibilityRole="button"
                    accessibilityLabel={`Increase ${stepName}`}
                    className={`items-center justify-center rounded-full border border-border bg-background ${stepSize} ${canIncrement ? 'opacity-100' : 'opacity-40'}`}
                >
                    <Plus size={16} color={iconColor} />
                </Pressable>
            </View>
            {hint && !hasError ? <Text className="text-xs text-muted">{hint}</Text> : null}
            {hasError && !hideError ? (
                <Text className="text-xs text-danger">{error.message}</Text>
            ) : null}
        </View>
    )
}
