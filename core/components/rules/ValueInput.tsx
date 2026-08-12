import { MenuActionItem } from '@tinycld/core/components/DropdownMenu'
import type { CatalogField } from '@tinycld/core/lib/automation/api'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Menu } from '@tinycld/core/ui/menu'
import { PlainInput } from '@tinycld/core/ui/PlainInput'
import { ChevronDown } from 'lucide-react-native'
import { Pressable, Text } from 'react-native'
import { RelationRecordPicker } from './RelationRecordPicker'

export interface ValueInputProps {
    field: CatalogField
    op: string
    value: unknown
    onChange: (v: string | number | boolean) => void
}

// `within_last_days` is a numeric "N days ago" value regardless of the
// field's own type (it's always a date field, but the value itself is a
// plain number of days) — so it's checked ahead of the field-type switch.
function inputKind(field: CatalogField, op: string): CatalogField['type'] | 'days' {
    if (op === 'within_last_days') return 'days'
    return field.type
}

function TextValueInput({
    value,
    onChange,
}: {
    value: unknown
    onChange: ValueInputProps['onChange']
}) {
    return (
        <PlainInput
            value={typeof value === 'string' ? value : ''}
            onChangeText={onChange}
            className="flex-1 border rounded-lg px-2.5 py-1.5 text-sm text-foreground bg-background border-border"
        />
    )
}

function NumberValueInput({
    value,
    onChange,
}: {
    value: unknown
    onChange: ValueInputProps['onChange']
}) {
    const placeholderColor = useThemeColor('field-placeholder')

    const handleChangeText = (text: string) => {
        if (text === '' || text === '-') {
            onChange(text)
            return
        }
        const digitsOnly = text.replace(/[^0-9-]/g, '')
        if (digitsOnly !== text) return
        const numValue = Number.parseInt(digitsOnly, 10)
        if (!Number.isNaN(numValue)) onChange(numValue)
    }

    return (
        <PlainInput
            value={value === '' || value === '-' ? value : String(value ?? '')}
            onChangeText={handleChangeText}
            keyboardType="numeric"
            placeholderTextColor={placeholderColor}
            className="flex-1 border rounded-lg px-2.5 py-1.5 text-sm text-foreground bg-background border-border"
        />
    )
}

function DateValueInput({
    value,
    onChange,
}: {
    value: unknown
    onChange: ValueInputProps['onChange']
}) {
    const placeholderColor = useThemeColor('field-placeholder')
    return (
        <PlainInput
            value={typeof value === 'string' ? value : ''}
            onChangeText={onChange}
            placeholder="YYYY-MM-DD"
            placeholderTextColor={placeholderColor}
            className="flex-1 border rounded-lg px-2.5 py-1.5 text-sm text-foreground bg-background border-border"
        />
    )
}

function SelectValueInput({
    field,
    value,
    onChange,
}: {
    field: CatalogField
    value: unknown
    onChange: ValueInputProps['onChange']
}) {
    const mutedColor = useThemeColor('muted-foreground')
    const options = field.options ?? []
    const selectedLabel = typeof value === 'string' && value ? value : 'Select…'

    return (
        <Menu>
            <Menu.Trigger>
                <Pressable className="flex-1 flex-row items-center justify-between border rounded-lg px-2.5 py-1.5 border-border bg-background">
                    <Text className="text-sm text-foreground" numberOfLines={1}>
                        {selectedLabel}
                    </Text>
                    <ChevronDown size={14} color={mutedColor} />
                </Pressable>
            </Menu.Trigger>
            <Menu.Portal>
                <Menu.Overlay />
                <Menu.Content presentation="popover" placement="bottom" align="start">
                    {options.map(option => (
                        <MenuActionItem
                            key={option}
                            label={option}
                            isActive={value === option}
                            onPress={() => onChange(option)}
                        />
                    ))}
                </Menu.Content>
            </Menu.Portal>
        </Menu>
    )
}

export function ValueInput({ field, op, value, onChange }: ValueInputProps) {
    const kind = inputKind(field, op)

    if (kind === 'days') return <NumberValueInput value={value} onChange={onChange} />
    if (kind === 'text') return <TextValueInput value={value} onChange={onChange} />
    if (kind === 'number') return <NumberValueInput value={value} onChange={onChange} />
    if (kind === 'date') return <DateValueInput value={value} onChange={onChange} />
    if (kind === 'select')
        return <SelectValueInput field={field} value={value} onChange={onChange} />
    if (kind === 'relation') {
        return (
            <RelationRecordPicker
                target={field.relationTarget ?? ''}
                displayField={field.displayField ?? 'id'}
                value={typeof value === 'string' ? value : ''}
                onChange={onChange}
            />
        )
    }
    // boolean ops (is_true/is_false) carry no value — caller already hides
    // ValueInput for NO_VALUE_OPS members, this is an unreachable fallback.
    return null
}
