import { MenuActionItem } from '@tinycld/core/components/DropdownMenu'
import type { CatalogField } from '@tinycld/core/lib/automation/api'
import { operatorLabel, operatorsForField } from '@tinycld/core/lib/automation/condition-helpers'
import { NO_VALUE_OPS } from '@tinycld/core/lib/automation/helpers'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Menu } from '@tinycld/core/ui/menu'
import { ChevronDown, Trash2 } from 'lucide-react-native'
import { Pressable, Text, View } from 'react-native'
import { ValueInput } from './ValueInput'

export interface ConditionRowCondition {
    field: string
    op: string
    value?: string | number | boolean
}

export interface ConditionRowProps {
    condition: ConditionRowCondition
    fields: CatalogField[]
    onChange: (patch: Partial<ConditionRowCondition>) => void
    onRemove: () => void
}

function FieldMenu({
    condition,
    fields,
    onSelectField,
}: {
    condition: ConditionRowCondition
    fields: CatalogField[]
    onSelectField: (field: CatalogField) => void
}) {
    const mutedColor = useThemeColor('muted-foreground')
    const selected = fields.find(f => f.key === condition.field)

    return (
        <Menu>
            <Menu.Trigger>
                <Pressable className="flex-1 flex-row items-center justify-between border rounded-lg px-2.5 py-1.5 border-border bg-background">
                    <Text className="text-sm text-foreground" numberOfLines={1}>
                        {selected?.label ?? 'Field…'}
                    </Text>
                    <ChevronDown size={14} color={mutedColor} />
                </Pressable>
            </Menu.Trigger>
            <Menu.Portal>
                <Menu.Overlay />
                <Menu.Content presentation="popover" placement="bottom" align="start">
                    {fields.map(field => (
                        <MenuActionItem
                            key={field.key}
                            label={field.label}
                            isActive={field.key === condition.field}
                            onPress={() => onSelectField(field)}
                        />
                    ))}
                </Menu.Content>
            </Menu.Portal>
        </Menu>
    )
}

function OperatorMenu({
    condition,
    selectedField,
    onSelectOp,
}: {
    condition: ConditionRowCondition
    selectedField: CatalogField | undefined
    onSelectOp: (op: string) => void
}) {
    const mutedColor = useThemeColor('muted-foreground')
    const ops = selectedField ? operatorsForField(selectedField) : []

    return (
        <Menu>
            <Menu.Trigger>
                <Pressable className="flex-1 flex-row items-center justify-between border rounded-lg px-2.5 py-1.5 border-border bg-background">
                    <Text className="text-sm text-foreground" numberOfLines={1}>
                        {condition.op ? operatorLabel(condition.op) : 'Operator…'}
                    </Text>
                    <ChevronDown size={14} color={mutedColor} />
                </Pressable>
            </Menu.Trigger>
            <Menu.Portal>
                <Menu.Overlay />
                <Menu.Content presentation="popover" placement="bottom" align="start">
                    {ops.map(op => (
                        <MenuActionItem
                            key={op}
                            label={operatorLabel(op)}
                            isActive={op === condition.op}
                            onPress={() => onSelectOp(op)}
                        />
                    ))}
                </Menu.Content>
            </Menu.Portal>
        </Menu>
    )
}

export function ConditionRow({ condition, fields, onChange, onRemove }: ConditionRowProps) {
    const mutedColor = useThemeColor('muted-foreground')
    const selectedField = fields.find(f => f.key === condition.field)
    const showValue = condition.op && !NO_VALUE_OPS.has(condition.op as never)

    const handleSelectField = (field: CatalogField) => {
        // Changing the field invalidates the operator (the old one may not
        // apply to the new field's type) and any value already entered.
        const legalOps = operatorsForField(field)
        onChange({ field: field.key, op: legalOps[0] ?? '', value: undefined })
    }

    const handleSelectOp = (op: string) => {
        // An op switch to a value-less op (is_true/is_false/is_empty) must
        // drop any value already entered — it would otherwise be silently
        // ignored by the engine while lingering in the draft.
        onChange({ op, value: NO_VALUE_OPS.has(op as never) ? undefined : condition.value })
    }

    return (
        <View className="flex-row items-center gap-2">
            <FieldMenu condition={condition} fields={fields} onSelectField={handleSelectField} />
            <OperatorMenu
                condition={condition}
                selectedField={selectedField}
                onSelectOp={handleSelectOp}
            />
            {showValue && selectedField ? (
                <ValueInput
                    field={selectedField}
                    op={condition.op}
                    value={condition.value}
                    onChange={v => onChange({ value: v })}
                />
            ) : null}
            <Pressable onPress={onRemove} className="p-1.5" hitSlop={8}>
                <Trash2 size={14} color={mutedColor} />
            </Pressable>
        </View>
    )
}
