import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import type { ComponentType } from 'react'
import { type Control, type FieldValues, type Path, useController } from 'react-hook-form'
import type { TextInputProps as RNTextInputProps } from 'react-native'
import { TextInput as RNTextInput, Text, View, type ViewProps } from 'react-native'

function LabelRow({
    label,
    icon: Icon,
}: {
    label: string
    icon?: ComponentType<{ size: number; color: string }>
}) {
    const mutedColor = useThemeColor('muted-foreground')
    if (!Icon) {
        return <Text className="text-sm font-semibold text-foreground">{label}</Text>
    }
    return (
        <View className="flex-row gap-2 items-center">
            <Icon size={16} color={mutedColor} />
            <Text className="text-sm font-semibold text-foreground shrink min-w-0">{label}</Text>
        </View>
    )
}

export type TextAreaInputProps<T extends FieldValues = Record<string, unknown>> = Omit<
    RNTextInputProps,
    | 'value'
    | 'onChangeText'
    | 'onBlur'
    | 'className'
    | 'style'
    | 'placeholderTextColor'
    | 'defaultValue'
    | 'multiline'
> & {
    name: Path<T>
    control: Control<T>
    label?: string
    labelIcon?: ComponentType<{ size: number; color: string }>
    hint?: string
    /** The caller renders the error itself (a form cell does), so the input only colours its border. */
    hideError?: boolean
    numberOfLines?: number
    wrapperProps?: ViewProps
}

export function TextAreaInput<T extends FieldValues = Record<string, unknown>>(
    props: TextAreaInputProps<T>
) {
    const {
        label,
        labelIcon: LabelIcon,
        hint,
        hideError = false,
        name,
        control,
        numberOfLines = 4,
        wrapperProps = {},
        testID,
        accessibilityLabel,
        ...inputProps
    } = props

    const {
        field,
        fieldState: { error },
    } = useController({ name, control })

    const placeholderColor = useThemeColor('field-placeholder')

    const hasError = !!error

    return (
        <View className="gap-1.5 mb-3" {...wrapperProps}>
            {label ? <LabelRow label={label} icon={LabelIcon} /> : null}
            <RNTextInput
                // Spread before the owned props for the same reason as TextInput.
                {...inputProps}
                multiline
                numberOfLines={numberOfLines}
                value={field.value || ''}
                onChangeText={field.onChange}
                onBlur={field.onBlur}
                accessibilityLabel={accessibilityLabel ?? label}
                testID={testID ?? name}
                placeholderTextColor={placeholderColor}
                textAlignVertical="top"
                className={`border rounded-lg px-3 py-2.5 text-base text-foreground bg-background ${hasError ? 'border-danger' : 'border-border'}`}
                style={{ minHeight: numberOfLines * 24 }}
            />
            {hint && !hasError ? <Text className="text-xs text-muted">{hint}</Text> : null}
            {hasError && !hideError ? (
                <Text className="text-xs text-danger">{error.message}</Text>
            ) : null}
        </View>
    )
}
