import { like } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { collectionByName } from '@tinycld/core/lib/pocketbase'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useDebouncedValue } from '@tinycld/core/lib/use-debounced-value'
import { Menu } from '@tinycld/core/ui/menu'
import { PlainInput } from '@tinycld/core/ui/PlainInput'
import { ChevronDown } from 'lucide-react-native'
import { useState } from 'react'
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
// useMyLiveQuery) when it isn't, rather than conditionally skipping the
// useLiveQuery call — hooks must run unconditionally.
//
// Raw useLiveQuery (not useMyLiveQuery) is correct here, same rationale as
// use-packages.ts: the collection may be a global, non-org-scoped store (or
// belong to another package entirely), so org/user scoping is the caller's
// concern, not this generic picker's — rows are already RLS-filtered by the
// server.

// How many matches the menu will render at once — and, because both the filter
// and the cap are now in the request, how many rows are ever fetched.
const VISIBLE_LIMIT = 50
const SEARCH_DEBOUNCE_MS = 250

// The target collection is named at runtime by a package's automation catalog, so
// `displayField` cannot be a key of any one row type — it is a key of whichever
// of the registered row types `target` resolved to. The ref proxy does answer an
// arbitrary property; only the static union of row shapes refuses to be indexed
// by a plain string. `string` is the narrowest type both `like` and `orderBy`
// accept for a value expression, so the cast lands there rather than on `any`,
// and it covers exactly this one access.
function fieldRef(record: object, displayField: string): string {
    return (record as Record<string, string>)[displayField]
}

function useRelationRecords(target: string, displayField: string, search: string) {
    const collection = collectionByName(target)
    const { data } = useLiveQuery({
        query: q => {
            if (!collection) return null
            // Both the filter and the cap belong in the request. The target is an
            // arbitrary collection a package's catalog named, so it could be the
            // deployment's largest table — an unfiltered read here was the only
            // one in the app whose worst case is unbounded. `like` compiles to
            // `<field> ~ "%<term>%"`; the explicit `%` on both sides is what
            // makes it a contains match, since PocketBase only auto-wraps an
            // operand that carries no `%` of its own (`wrapLikeParams`). The
            // order is the display field rather than id because id order is
            // meaningless to a human.
            let query = q.from({ record: collection })
            if (search) {
                query = query.where(({ record }) =>
                    like(fieldRef(record, displayField), `%${search}%`)
                )
            }
            return query
                .orderBy(({ record }) => fieldRef(record, displayField))
                .limit(VISIBLE_LIMIT)
        },
    })
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
    const [search, setSearch] = useState('')
    // Debounced so each keystroke does not open a new subscription — the query's
    // identity is derived from the term it captures. Backslashes are dropped
    // because pbtsdb's escapeValue escapes `"` but not `\`, so a term containing
    // `\"` compiles to an unterminated filter literal and PocketBase answers 400.
    const debouncedSearch = useDebouncedValue(search.trim().replace(/\\/g, ''), SEARCH_DEBOUNCE_MS)
    const { isRegistered, records } = useRelationRecords(target, displayField, debouncedSearch)

    const matches = records.map(record => ({ record, label: recordLabel(record, displayField) }))

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

    // The trigger label falls back to the raw id when the selected record is not
    // in the current window: the search narrows what the menu lists, and the
    // selection is frequently filtered out of it. Reading it off a full
    // collection fetch is what this component is no longer allowed to do.
    const selected = matches.find(entry => entry.record.id === value)
    const label = selected ? selected.label : value || 'Select…'

    return (
        <Menu
            trigger={
                <Pressable className="flex-1 flex-row items-center justify-between border rounded-lg px-2.5 py-1.5 border-border bg-background">
                    <Text className="text-sm text-foreground" numberOfLines={1}>
                        {label}
                    </Text>
                    <ChevronDown size={14} color={mutedColor} />
                </Pressable>
            }
            placement="bottom-start"
        >
            <Menu.Custom className="px-2 pt-1 pb-2">
                <PlainInput
                    value={search}
                    onChangeText={setSearch}
                    placeholder="Search…"
                    placeholderTextColor={placeholderColor}
                    className="border rounded-lg px-2.5 py-1.5 text-sm text-foreground bg-background border-border"
                />
            </Menu.Custom>
            {matches.map(({ record, label: recordName }) => (
                <Menu.Item
                    key={record.id as string}
                    label={recordName}
                    isSelected={record.id === value}
                    onSelect={() => onChange(record.id as string)}
                />
            ))}
            <EmptyHint isVisible={matches.length === 0} hasSearch={Boolean(debouncedSearch)} />
            <MoreHint isVisible={matches.length >= VISIBLE_LIMIT} />
        </Menu>
    )
}

function EmptyHint({ isVisible, hasSearch }: { isVisible: boolean; hasSearch: boolean }) {
    if (!isVisible) return null
    return (
        <Text className="px-3 py-2 text-xs text-muted-foreground">
            {hasSearch ? 'No matches' : 'Nothing to choose from'}
        </Text>
    )
}

// Say so rather than silently truncating: a user who can't see their record
// needs to know to narrow the search, not assume it doesn't exist. A full window
// is the only signal available — the server was not asked how many more there
// are, deliberately, since counting costs a second unbounded query.
function MoreHint({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return (
        <Text className="px-3 py-2 text-xs text-muted-foreground">
            Showing the first {VISIBLE_LIMIT} — keep typing to narrow
        </Text>
    )
}
