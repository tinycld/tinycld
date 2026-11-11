import { type Control, type FieldValues, type Path, useController, useWatch } from 'react-hook-form'
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
    /** The caller renders the error itself (a form cell does), so the input only colours its border. */
    hideError?: boolean
    disabled?: boolean
    /** `segmented` lays short labels side by side; `list` stacks them with descriptions. */
    layout?: 'list' | 'segmented'
    /**
     * The value of the option that stands for "no value": choosing it writes
     * undefined into the form, and it is selected while the field has no value.
     */
    clearValue?: string
    wrapperProps?: ViewProps
}

const SEGMENT_CLASS = {
    selected:
        'min-w-16 shrink-0 basis-auto items-center rounded-md border px-3 py-2 border-primary bg-primary',
    idle: 'min-w-16 shrink-0 basis-auto items-center rounded-md border px-3 py-2 border-border bg-background',
} as const

const SEGMENT_TEXT_CLASS = {
    selected: 'text-sm font-medium text-primary-foreground',
    idle: 'text-sm font-medium text-foreground',
} as const

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
    hideError = false,
    disabled = false,
    layout = 'list',
    clearValue,
    wrapperProps = {},
}: RadioInputProps<T>) {
    const {
        field,
        fieldState: { error },
    } = useController({ name, control })
    // A controller falls back to the loaded value when the form value turns
    // undefined, so a field that can be cleared is read through useWatch.
    const watched = useWatch({ control, name })
    const value = clearValue === undefined ? field.value : (watched ?? clearValue)
    const hasError = !!error
    const isSegmented = layout === 'segmented'
    const choose = (choice: string) => field.onChange(choice === clearValue ? undefined : choice)
    const rows = options.map(option => {
        const isSelected = value === option.value
        const onPress = () => choose(option.value)
        if (isSegmented) {
            const state = isSelected ? 'selected' : 'idle'
            // A segment keeps its label whole on one line; the row wraps when they do not fit.
            return (
                <Pressable
                    key={option.value}
                    testID={`${name}-${option.value}`}
                    accessibilityRole="radio"
                    accessibilityState={{ checked: isSelected, disabled }}
                    disabled={disabled}
                    accessibilityLabel={option.label}
                    onPress={onPress}
                    className={SEGMENT_CLASS[state]}
                >
                    <Text className={SEGMENT_TEXT_CLASS[state]} numberOfLines={1}>
                        {option.label}
                    </Text>
                </Pressable>
            )
        }
        return (
            <Pressable
                key={option.value}
                testID={`${name}-${option.value}`}
                accessibilityRole="radio"
                accessibilityState={{ checked: isSelected, disabled }}
                disabled={disabled}
                accessibilityLabel={option.label}
                onPress={onPress}
                className="flex-row items-start gap-3 py-1.5"
            >
                <View className="pt-0.5">
                    <RadioDot isSelected={isSelected} />
                </View>
                <View className="flex-1 min-w-0 gap-0.5">
                    <Text className="text-sm font-medium text-foreground">{option.label}</Text>
                    <Description text={option.description} />
                </View>
            </Pressable>
        )
    })
    return (
        <View className="mb-3 gap-1.5" {...wrapperProps}>
            <Text className="text-sm font-semibold text-foreground">{label}</Text>
            <View
                accessibilityRole="radiogroup"
                accessibilityLabel={label}
                className={isSegmented ? 'flex-row flex-wrap gap-1.5' : ''}
            >
                {rows}
            </View>
            {hint && !hasError ? <Text className="text-xs text-muted">{hint}</Text> : null}
            {hasError && !hideError ? (
                <Text className="text-xs text-danger">{error.message}</Text>
            ) : null}
        </View>
    )
}
