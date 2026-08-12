import { useLiveQuery } from '@tanstack/react-db'
import { MenuActionItem } from '@tinycld/core/components/DropdownMenu'
import { collectionByName } from '@tinycld/core/lib/pocketbase'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Menu } from '@tinycld/core/ui/menu'
import { PlainInput } from '@tinycld/core/ui/PlainInput'
import { ChevronDown } from 'lucide-react-native'
import { Pressable, Text } from 'react-native'

export interface RelationRecordPickerProps {
    target: string
    displayField: string
    value: string
    onChange: (id: string) => void
}

function recordLabel(record: Record<string, unknown>, displayField: string): string {
    const raw = record[displayField]
    const id = record.id
    return typeof raw === 'string' && raw ? raw : typeof id === 'string' ? id : ''
}

// Relation targets are arbitrary collections declared by a package's
// automation catalog — not guaranteed to be registered in the client's
// pbtsdb store map (an unlinked package, or a name outside the tinycld
// schema entirely). collectionByName resolves it dynamically when it IS
// registered; the query itself no-ops (returns null, same pattern as
// useOrgLiveQuery) when it isn't, rather than conditionally skipping the
// useLiveQuery call — hooks must run unconditionally.
//
// Raw useLiveQuery (not useOrgLiveQuery) is correct here, same rationale as
// use-packages.ts: the collection may be a global, non-org-scoped store (or
// belong to another package entirely), so org/user scoping is the caller's
// concern, not this generic picker's — rows are already RLS-filtered by the
// server.
function useRelationRecords(target: string) {
    const collection = collectionByName(target)
    const { data } = useLiveQuery(
        q => {
            if (!collection) return null
            return q.from({ record: collection }).limit(50)
        },
        [collection]
    )
    return { isRegistered: Boolean(collection), records: (data ?? []) as Record<string, unknown>[] }
}

export function RelationRecordPicker({
    target,
    displayField,
    value,
    onChange,
}: RelationRecordPickerProps) {
    const mutedColor = useThemeColor('muted-foreground')
    const { isRegistered, records } = useRelationRecords(target)

    if (!isRegistered) {
        return (
            <PlainInput
                value={value}
                onChangeText={onChange}
                placeholder={`record id — ${target} isn't installed here`}
                className="flex-1 border rounded-lg px-2.5 py-1.5 text-sm text-foreground bg-background border-border"
            />
        )
    }

    const selected = records.find(r => r.id === value)
    const label = selected ? recordLabel(selected, displayField) : value || 'Select…'

    return (
        <Menu>
            <Menu.Trigger>
                <Pressable className="flex-1 flex-row items-center justify-between border rounded-lg px-2.5 py-1.5 border-border bg-background">
                    <Text className="text-sm text-foreground" numberOfLines={1}>
                        {label}
                    </Text>
                    <ChevronDown size={14} color={mutedColor} />
                </Pressable>
            </Menu.Trigger>
            <Menu.Portal>
                <Menu.Overlay />
                <Menu.Content presentation="popover" placement="bottom" align="start">
                    {records.map(record => (
                        <MenuActionItem
                            key={record.id as string}
                            label={recordLabel(record, displayField)}
                            isActive={record.id === value}
                            onPress={() => onChange(record.id as string)}
                        />
                    ))}
                </Menu.Content>
            </Menu.Portal>
        </Menu>
    )
}
