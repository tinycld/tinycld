import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Check } from 'lucide-react-native'
import { type Control, type FieldValues, type Path, useController } from 'react-hook-form'
import { Pressable, Text, View, type ViewProps } from 'react-native'
import type { SelectOption } from './ButtonGroup'

export type MultiSelectInputProps<T extends FieldValues = Record<string, unknown>> = {
    name: Path<T>
    control: Control<T>
    label: string
    options: SelectOption[]
    hint?: string
    emptyText?: string
    wrapperProps?: ViewProps
}

// Keeps the stored order equal to the options' order, so the same choice
// always saves the same array.
function toggled(options: SelectOption[], current: string[], value: string) {
    const next = new Set(current)
    if (next.has(value)) next.delete(value)
    else next.add(value)
    return options.map(o => o.value).filter(v => next.has(v))
}

export function MultiSelectInput<T extends FieldValues = Record<string, unknown>>({
    name,
    control,
    label,
    options,
    hint,
    emptyText,
    wrapperProps = {},
}: MultiSelectInputProps<T>) {
    const {
        field,
        fieldState: { error },
    } = useController({ name, control })
    const selected: string[] = Array.isArray(field.value) ? field.value : []
    const onToggle = (value: string) => field.onChange(toggled(options, selected, value))

    return (
        <View className="gap-1.5 mb-3" {...wrapperProps}>
            <Text className="text-sm font-semibold text-foreground">{label}</Text>
            <View className="flex-col gap-1">
                {options.map(option => (
                    <MultiSelectRow
                        key={option.value}
                        label={option.label}
                        isChecked={selected.includes(option.value)}
                        onPress={() => onToggle(option.value)}
                    />
                ))}
            </View>
            <FieldNote
                isVisible={options.length === 0 && !!emptyText}
                text={emptyText ?? ''}
                tone="muted"
            />
            <FieldNote isVisible={!!hint && !error} text={hint ?? ''} tone="muted" />
            <FieldNote isVisible={!!error} text={error?.message ?? ''} tone="danger" />
        </View>
    )
}

function MultiSelectRow({
    label,
    isChecked,
    onPress,
}: {
    label: string
    isChecked: boolean
    onPress: () => void
}) {
    const primaryFg = useThemeColor('primary-foreground')
    const boxClass = isChecked ? 'border-primary bg-primary' : 'border-border bg-transparent'
    return (
        <Pressable
            accessibilityRole="checkbox"
            accessibilityLabel={label}
            accessibilityState={{ checked: isChecked }}
            aria-checked={isChecked}
            onPress={onPress}
            className="flex-row items-center gap-2 py-1.5"
        >
            <View className={`w-4 h-4 rounded border items-center justify-center ${boxClass}`}>
                <CheckMark isVisible={isChecked} color={primaryFg} />
            </View>
            <Text className="shrink min-w-0 text-sm text-foreground">{label}</Text>
        </Pressable>
    )
}

function CheckMark({ isVisible, color }: { isVisible: boolean; color: string }) {
    if (!isVisible) return null
    return <Check size={12} color={color} strokeWidth={3} />
}

function FieldNote({
    isVisible,
    text,
    tone,
}: {
    isVisible: boolean
    text: string
    tone: 'muted' | 'danger'
}) {
    if (!isVisible) return null
    return (
        <Text className={tone === 'danger' ? 'text-xs text-danger' : 'text-xs text-muted'}>
            {text}
        </Text>
    )
}
