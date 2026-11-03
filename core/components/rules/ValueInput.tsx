import type { CatalogField } from '@tinycld/core/lib/automation/api'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Menu } from '@tinycld/core/ui/menu'
import { PlainInput } from '@tinycld/core/ui/PlainInput'
import { ChevronDown } from 'lucide-react-native'
import { useState } from 'react'
import { Pressable, Text } from 'react-native'
import { RelationRecordPicker } from './RelationRecordPicker'

export interface ValueInputProps {
    field: CatalogField
    op: string
    value: unknown
    // `undefined` means "no usable value right now" — a numeric field mid-edit
    // ("" or "-") reports it rather than storing a non-numeric string that
    // would make the condition fail closed forever.
    onChange: (v: string | number | boolean | undefined) => void
    /** Names a typed input for a screen reader, e.g. "Condition value". */
    accessibilityLabel?: string
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
    accessibilityLabel,
}: {
    value: unknown
    onChange: ValueInputProps['onChange']
    accessibilityLabel?: string
}) {
    return (
        <PlainInput
            value={typeof value === 'string' ? value : ''}
            onChangeText={onChange}
            accessibilityLabel={accessibilityLabel}
            className="flex-1 border rounded-lg px-2.5 py-1.5 text-sm text-foreground bg-background border-border"
        />
    )
}

function NumberValueInput({
    value,
    onChange,
    accessibilityLabel,
}: {
    value: unknown
    onChange: ValueInputProps['onChange']
    accessibilityLabel?: string
}) {
    const placeholderColor = useThemeColor('field-placeholder')
    // While typing, "" and "-" are legitimate intermediate states that are not
    // numbers. They stay local: propagating them would store a non-numeric
    // value in the condition, and the engine's numeric operators fail closed on
    // anything toFloat can't read — so the rule would silently never match
    // again, even after the user typed a real number elsewhere.
    const [draft, setDraft] = useState<string | null>(null)

    const handleChangeText = (text: string) => {
        if (text === '' || text === '-') {
            setDraft(text)
            onChange(undefined)
            return
        }
        const digitsOnly = text.replace(/[^0-9-]/g, '')
        if (digitsOnly !== text) return
        const numValue = Number.parseInt(digitsOnly, 10)
        if (Number.isFinite(numValue)) {
            setDraft(null)
            onChange(numValue)
        }
    }

    return (
        <PlainInput
            value={draft ?? (typeof value === 'number' ? String(value) : '')}
            onChangeText={handleChangeText}
            accessibilityLabel={accessibilityLabel}
            keyboardType="numeric"
            placeholderTextColor={placeholderColor}
            className="flex-1 border rounded-lg px-2.5 py-1.5 text-sm text-foreground bg-background border-border"
        />
    )
}

function DateValueInput({
    value,
    onChange,
    accessibilityLabel,
}: {
    value: unknown
    onChange: ValueInputProps['onChange']
    accessibilityLabel?: string
}) {
    const placeholderColor = useThemeColor('field-placeholder')
    return (
        <PlainInput
            value={typeof value === 'string' ? value : ''}
            onChangeText={onChange}
            accessibilityLabel={accessibilityLabel}
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
        <Menu
            trigger={
                <Pressable
                    accessibilityLabel="Condition value"
                    className="flex-1 flex-row items-center justify-between border rounded-lg px-2.5 py-1.5 border-border bg-background"
                >
                    <Text className="text-sm text-foreground" numberOfLines={1}>
                        {selectedLabel}
                    </Text>
                    <ChevronDown size={14} color={mutedColor} />
                </Pressable>
            }
            placement="bottom-start"
        >
            {options.map(option => (
                <Menu.Item
                    key={option}
                    label={option}
                    isSelected={value === option}
                    onSelect={() => onChange(option)}
                />
            ))}
        </Menu>
    )
}

export function ValueInput({ field, op, value, onChange, accessibilityLabel }: ValueInputProps) {
    const kind = inputKind(field, op)
    const typed = { value, onChange, accessibilityLabel }

    if (kind === 'days') return <NumberValueInput {...typed} />
    if (kind === 'text') return <TextValueInput {...typed} />
    if (kind === 'number') return <NumberValueInput {...typed} />
    if (kind === 'date') return <DateValueInput {...typed} />
    if (kind === 'select')
        return <SelectValueInput field={field} value={value} onChange={onChange} />
    if (kind === 'relation') {
        return (
            <RelationRecordPicker
                target={field.relationTarget ?? ''}
                displayField={field.displayField ?? 'id'}
                value={typeof value === 'string' ? value : ''}
                onChange={onChange}
                match={field.relationMatch}
                create={field.relationCreate}
                accessibilityLabel="Condition value"
            />
        )
    }
    // boolean ops (is_true/is_false) carry no value — caller already hides
    // ValueInput for NO_VALUE_OPS members, this is an unreachable fallback.
    return null
}
