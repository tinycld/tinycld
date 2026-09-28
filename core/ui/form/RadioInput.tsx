import { type Control, type FieldValues, type Path, useController } from 'react-hook-form'
import { Pressable, Text, View, type ViewProps } from 'react-native'

export type RadioOption = {
    label: string
    value: string
    /** One line under the label saying what choosing it means. */
    description?: string
}

export type RadioInputProps<T extends FieldValues = Record<string, unknown>> = {
    name: Path<T>
    control: Control<T>
    label: string
    options: RadioOption[]
    hint?: string
    wrapperProps?: ViewProps
}

const DOT_CLASS = {
    selected: 'size-5 items-center justify-center rounded-full border-2 border-primary',
    idle: 'size-5 items-center justify-center rounded-full border-2 border-border',
} as const

function RadioDot({ isSelected }: { isSelected: boolean }) {
    if (!isSelected) return <View className={DOT_CLASS.idle} />
    return (
        <View className={DOT_CLASS.selected}>
            <View className="size-2.5 rounded-full bg-primary" />
        </View>
    )
}

function Description({ text }: { text: string | undefined }) {
    if (!text) return null
    return <Text className="text-xs leading-4 text-muted-foreground">{text}</Text>
}

/**
 * One choice from a short list, each option on its own row with a radio dot
 * and an optional description. For a choice that is a mode or a role, where
 * the person needs to read what each option means before picking.
 */
export function RadioInput<T extends FieldValues = Record<string, unknown>>({
    name,
    control,
    label,
    options,
    hint,
    wrapperProps = {},
}: RadioInputProps<T>) {
    const {
        field,
        fieldState: { error },
    } = useController({ name, control })
    const hasError = !!error
    const rows = options.map(option => {
        const isSelected = field.value === option.value
        return (
            <Pressable
                key={option.value}
                testID={`${name}-${option.value}`}
                accessibilityRole="radio"
                accessibilityState={{ checked: isSelected }}
                accessibilityLabel={option.label}
                onPress={() => field.onChange(option.value)}
                className="flex-row items-start gap-3 py-1.5"
            >
                <View className="pt-0.5">
                    <RadioDot isSelected={isSelected} />
                </View>
                <View className="flex-1 gap-0.5">
                    <Text className="text-sm font-medium text-foreground">{option.label}</Text>
                    <Description text={option.description} />
                </View>
            </Pressable>
        )
    })
    return (
        <View className="mb-3 gap-1.5" {...wrapperProps}>
            <Text className="text-sm font-semibold text-foreground">{label}</Text>
            <View accessibilityRole="radiogroup">{rows}</View>
            {hint && !hasError ? <Text className="text-xs text-muted">{hint}</Text> : null}
            {hasError ? <Text className="text-xs text-danger">{error.message}</Text> : null}
        </View>
    )
}
