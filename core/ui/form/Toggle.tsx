import { Switch } from '@tinycld/core/ui/switch'
import { type Control, type FieldValues, type Path, useController } from 'react-hook-form'
import { Text, View, type ViewProps } from 'react-native'

export type ToggleProps<T extends FieldValues = Record<string, unknown>> = {
    name: Path<T>
    control: Control<T>
    label: string
    /** Keeps `label` as the switch's accessible name but does not draw it, as in a table cell under its header. */
    hideLabel?: boolean
    hint?: string
    /** The caller renders the error itself (a form cell does), so the input only colours its border. */
    hideError?: boolean
    disabled?: boolean
    /** Called after the form value changes, so a caller can clear dependent fields in the change event itself. */
    onValueChange?: (value: boolean) => void
    /** Defaults to `name`, as on the other form inputs. */
    testID?: string
    wrapperProps?: ViewProps
    /** Called when the switch takes focus (a table notes which row the keyboard is in). */
    onFocus?: () => void
    /** Called when the switch loses focus (a table forgets the row the keyboard left). */
    onBlur?: () => void
}

export function Toggle<T extends FieldValues = Record<string, unknown>>({
    name,
    control,
    label,
    hideLabel = false,
    hint,
    hideError = false,
    disabled = false,
    onValueChange,
    testID,
    wrapperProps = {},
    onFocus,
    onBlur,
}: ToggleProps<T>) {
    const {
        field,
        fieldState: { error },
    } = useController({ name, control })

    const hasError = !!error

    return (
        <View className="gap-1.5 mb-3" {...wrapperProps}>
            <View className="flex-row items-center justify-between gap-2">
                {hideLabel ? null : (
                    <Text className="text-sm font-semibold text-foreground">{label}</Text>
                )}
                <Switch
                    value={Boolean(field.value)}
                    onValueChange={next => {
                        field.onChange(next)
                        onValueChange?.(next)
                    }}
                    disabled={disabled}
                    onFocus={onFocus}
                    onBlur={onBlur}
                    accessibilityLabel={label}
                    testID={testID ?? name}
                />
            </View>
            {hint && !hasError ? <Text className="text-xs text-muted">{hint}</Text> : null}
            {hasError && !hideError ? (
                <Text className="text-xs text-danger">{error.message}</Text>
            ) : null}
        </View>
    )
}
