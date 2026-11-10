import { RelationRecordPicker } from '@tinycld/core/components/rules/RelationRecordPicker'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useRecord } from '@tinycld/core/lib/use-record'
import { X } from 'lucide-react-native'
import { Pressable, Text, View } from 'react-native'

export interface UserFieldProps {
    value: string | string[] | null
    onChange: (value: string | string[] | null) => void
    multiple: boolean
    disabled?: boolean
    testID?: string
    accessibilityLabel?: string
}

function UserChip({
    id,
    onRemove,
    disabled,
}: {
    id: string
    onRemove: () => void
    disabled?: boolean
}) {
    const [users] = useStore('users')
    const { record } = useRecord(users, id)
    const fg = useThemeColor('foreground')
    return (
        <View className="max-w-full flex-row items-center bg-muted rounded-full px-2 py-0.5 mr-1 mb-1">
            <Text className="shrink min-w-0 text-foreground text-sm" numberOfLines={1}>
                {record?.name ?? '…'}
            </Text>
            <Pressable
                onPress={onRemove}
                disabled={disabled}
                accessibilityLabel={`Remove ${record?.name ?? 'user'}`}
                className="ml-1"
            >
                <X size={14} color={fg} />
            </Pressable>
        </View>
    )
}

function Chips({
    ids,
    onRemove,
    disabled,
}: {
    ids: string[]
    onRemove: (id: string) => void
    disabled?: boolean
}) {
    if (ids.length === 0) return null
    return (
        <View className="flex-row flex-wrap mb-1">
            {ids.map(id => (
                <UserChip key={id} id={id} onRemove={() => onRemove(id)} disabled={disabled} />
            ))}
        </View>
    )
}

function toIds(value: string | string[] | null): string[] {
    if (Array.isArray(value)) return value
    return value ? [value] : []
}

export function UserField({
    value,
    onChange,
    multiple,
    disabled,
    testID,
    accessibilityLabel,
}: UserFieldProps) {
    const ids = toIds(value)
    const add = (id: string) => {
        if (!id) return
        if (multiple) onChange(ids.includes(id) ? ids : [...ids, id])
        else onChange(id)
    }
    const remove = (id: string) => (multiple ? onChange(ids.filter(x => x !== id)) : onChange(null))
    return (
        <View testID={testID} accessibilityLabel={accessibilityLabel}>
            <Chips ids={ids} onRemove={remove} disabled={disabled} />
            <View pointerEvents={disabled ? 'none' : 'auto'}>
                <RelationRecordPicker
                    target="users"
                    displayField="name"
                    value={multiple ? '' : (ids[0] ?? '')}
                    onChange={add}
                />
            </View>
        </View>
    )
}
