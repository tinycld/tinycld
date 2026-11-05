import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Menu } from '@tinycld/core/ui/menu'
import { ChevronDown } from 'lucide-react-native'
import { Fragment } from 'react'
import { type Control, type FieldValues, type Path, useController } from 'react-hook-form'
import { Pressable, Text, View, type ViewProps } from 'react-native'
import type { SelectOption } from './ButtonGroup'
import { selectSections } from './select-sections'

export type SelectInputProps<T extends FieldValues = Record<string, unknown>> = {
    name: Path<T>
    control: Control<T>
    label: string
    /** Keeps `label` as the trigger's accessible name but does not draw it, as in a table cell under its header. */
    hideLabel?: boolean
    options: SelectOption[]
    hint?: string
    /** The caller renders the error itself (a form cell does), so the input only colours its border. */
    hideError?: boolean
    disabled?: boolean
    /** Called after the form value changes to a different option, so a caller can clear dependent fields in the change event itself. */
    onValueChange?: (value: string) => void
    wrapperProps?: ViewProps
    /** Shown in the trigger while no option matches the value. */
    placeholder?: string
    /** Item test ids are `${testID}-${value}`. */
    testID?: string
}

export function SelectInput<T extends FieldValues = Record<string, unknown>>({
    name,
    control,
    label,
    hideLabel = false,
    options,
    hint,
    hideError = false,
    disabled = false,
    onValueChange,
    wrapperProps = {},
    placeholder = '',
    testID,
}: SelectInputProps<T>) {
    const muted = useThemeColor('muted-foreground')
    const {
        field,
        fieldState: { error },
    } = useController({ name, control })

    const hasError = !!error
    const showError = hasError && !hideError
    const shown = options.find(option => option.value === field.value)?.label ?? placeholder

    const choose = (value: string) => {
        const changed = field.value !== value
        field.onChange(value)
        if (changed) onValueChange?.(value)
    }

    const itemOf = (option: SelectOption) => (
        <Menu.Item
            key={option.value}
            label={option.label}
            indent={option.indent}
            isSelected={option.value === field.value}
            onSelect={() => choose(option.value)}
            testID={testID ? `${testID}-${option.value}` : undefined}
        />
    )

    const menuRows = selectSections(options).map((run, index) => {
        const key = `run-${index}`
        if (run.label === null) return <Fragment key={key}>{run.options.map(itemOf)}</Fragment>
        return (
            <Menu.Section key={key} label={run.label}>
                {run.options.map(itemOf)}
            </Menu.Section>
        )
    })

    const trigger = (
        <Pressable
            className={`flex-row items-center justify-between gap-2 border rounded-md px-3 ${showError ? 'border-danger' : 'border-border'}`}
            style={{ minHeight: 44 }}
            accessibilityLabel={`${label}: ${shown}`}
            disabled={disabled}
            aria-disabled={disabled}
            accessibilityState={{ disabled }}
            testID={testID}
        >
            <Text className="text-foreground text-sm">{shown}</Text>
            <ChevronDown size={12} color={muted} />
        </Pressable>
    )

    return (
        <View className="gap-1.5 mb-3" {...wrapperProps}>
            {label && !hideLabel ? (
                <Text className="text-sm font-semibold text-foreground">{label}</Text>
            ) : null}
            <Menu presentation="popover" trigger={trigger}>
                {menuRows}
            </Menu>
            {hint && !hasError ? <Text className="text-xs text-muted">{hint}</Text> : null}
            {showError ? <Text className="text-xs text-danger">{error.message}</Text> : null}
        </View>
    )
}
