import { useLiveQuery } from '@tanstack/react-db'
import { MenuActionItem } from '@tinycld/core/components/DropdownMenu'
import { collectionByName } from '@tinycld/core/lib/pocketbase'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Menu } from '@tinycld/core/ui/menu'
import { PlainInput } from '@tinycld/core/ui/PlainInput'
import { ChevronDown } from 'lucide-react-native'
import { useMemo, useState } from 'react'
import { Pressable, Text, View } from 'react-native'

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
// How many matches the menu will render at once. The list is filtered before
// it is capped, so this bounds the DOM, not what the user can reach.
const VISIBLE_LIMIT = 50

function useRelationRecords(target: string) {
    const collection = collectionByName(target)
    const { data } = useLiveQuery(
        q => {
            if (!collection) return null
            // No LIMIT here: the cap has to be applied AFTER filtering, or the
            // filter only ever searches the first N rows. Ordering by id is
            // likewise wrong as a user-facing order — it's random to a human —
            // so both are handled below against the display field.
            return q.from({ record: collection })
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
    const placeholderColor = useThemeColor('field-placeholder')
    const { isRegistered, records } = useRelationRecords(target)
    const [search, setSearch] = useState('')

    // Labelling is independent of the search term, so it is memoized on its own
    // — otherwise every keystroke re-derived a label for every row.
    const labelled = useMemo(
        () =>
            records.map(record => ({
                record,
                label: recordLabel(record, displayField),
            })),
        [records, displayField]
    )

    // Filter FIRST, then sort. Sorting the whole collection before filtering
    // ran an O(n log n) pass per keystroke over every row, when the sort only
    // ever needs to order what survives the filter — usually a handful.
    //
    // Capping comes last, and after the sort rather than before it: ordering by
    // id and capping first (the previous behavior) meant a collection larger
    // than the cap hid most of itself behind an arbitrary boundary, so a folder
    // created a moment ago was typically unreachable with no way to search for
    // it.
    const matches = useMemo(() => {
        const needle = search.trim().toLowerCase()
        const filtered = needle
            ? labelled.filter(entry => entry.label.toLowerCase().includes(needle))
            : [...labelled]
        filtered.sort((a, b) => a.label.localeCompare(b.label))
        return { visible: filtered.slice(0, VISIBLE_LIMIT), total: filtered.length }
    }, [labelled, search])

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

    // Resolved against the full labelled set, not `matches` — the trigger must
    // keep showing the selected record's name while a search is narrowing the
    // list, and the selection is frequently filtered out of it.
    const selected = labelled.find(entry => entry.record.id === value)
    const label = selected ? selected.label : value || 'Select…'

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
            <Menu.Portal hasTextInput>
                <Menu.Overlay />
                <Menu.Content presentation="popover" placement="bottom" align="start">
                    <View className="px-2 pt-1 pb-2">
                        <PlainInput
                            value={search}
                            onChangeText={setSearch}
                            placeholder="Search…"
                            placeholderTextColor={placeholderColor}
                            className="border rounded-lg px-2.5 py-1.5 text-sm text-foreground bg-background border-border"
                        />
                    </View>
                    {matches.visible.map(({ record, label: recordName }) => (
                        <MenuActionItem
                            key={record.id as string}
                            label={recordName}
                            isActive={record.id === value}
                            onPress={() => onChange(record.id as string)}
                        />
                    ))}
                    {matches.visible.length === 0 ? (
                        <Text className="px-3 py-2 text-xs text-muted-foreground">
                            {records.length === 0 ? 'Nothing to choose from' : 'No matches'}
                        </Text>
                    ) : null}
                    {matches.total > matches.visible.length ? (
                        // Say so rather than silently truncating: a user who
                        // can't see their record needs to know to narrow the
                        // search, not assume it doesn't exist.
                        <Text className="px-3 py-2 text-xs text-muted-foreground">
                            {matches.total - matches.visible.length} more — keep typing to narrow
                        </Text>
                    ) : null}
                </Menu.Content>
            </Menu.Portal>
        </Menu>
    )
}
