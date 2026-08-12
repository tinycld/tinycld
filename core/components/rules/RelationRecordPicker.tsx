import { useQuery } from '@tanstack/react-query'
import { MenuActionItem } from '@tinycld/core/components/DropdownMenu'
import { pb } from '@tinycld/core/lib/pocketbase'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Menu } from '@tinycld/core/ui/menu'
import { ChevronDown } from 'lucide-react-native'
import { Pressable, Text } from 'react-native'

export interface RelationRecordPickerProps {
    target: string
    displayField: string
    value: string
    onChange: (id: string) => void
}

type PickerRecord = { id: string } & Record<string, unknown>

function recordLabel(record: PickerRecord, displayField: string): string {
    const raw = record[displayField]
    return typeof raw === 'string' && raw ? raw : record.id
}

// Relation targets are arbitrary collections declared by a package's
// automation catalog — not guaranteed to be registered in the client's
// pbtsdb store map (`useStore` is typed by literal collection name, so it
// can't be called with a dynamic `target: string`). A one-shot, read-only
// PB list fetch is the pragmatic fallback: it's allowed under the
// "read-only reads" exception since rule authoring only needs to *display*
// candidate records to pick from, not live-sync them.
function useRelationRecords(target: string) {
    return useQuery({
        queryKey: ['automation-relation-records', target],
        // biome-ignore lint/plugin/pbtsdb-no-raw-pb-access: target is an arbitrary collection from a package's automation catalog, not guaranteed to be in the pbtsdb store map (useStore can't be called dynamically) — no collection object exists to hand to readCollectionCached, so a one-shot read-only network call is the pragmatic fallback.
        queryFn: () => pb.collection(target).getList<PickerRecord>(1, 50),
        enabled: Boolean(target),
    })
}

export function RelationRecordPicker({
    target,
    displayField,
    value,
    onChange,
}: RelationRecordPickerProps) {
    const mutedColor = useThemeColor('muted-foreground')
    const { data } = useRelationRecords(target)
    const records = data?.items ?? []
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
                            key={record.id}
                            label={recordLabel(record, displayField)}
                            isActive={record.id === value}
                            onPress={() => onChange(record.id)}
                        />
                    ))}
                </Menu.Content>
            </Menu.Portal>
        </Menu>
    )
}
