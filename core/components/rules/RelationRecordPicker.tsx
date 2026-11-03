import { useAuth } from '@tinycld/core/lib/auth'
import type { RelationCreate } from '@tinycld/core/lib/automation/types'
import { useMutation } from '@tinycld/core/lib/mutations'
import { collectionByName, insertByName } from '@tinycld/core/lib/pocketbase'
import { fieldLabel, type RecordLabel } from '@tinycld/core/lib/record-labels/label'
import { packageRecordLabels } from '@tinycld/core/lib/record-labels/registry'
import {
    type LabelSources,
    useLabelledRecord,
    useLabelledRecords,
    VISIBLE_LIMIT,
} from '@tinycld/core/lib/record-labels/use-labelled-records'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useDebouncedValue } from '@tinycld/core/lib/use-debounced-value'
import { Menu } from '@tinycld/core/ui/menu'
import { PlainInput } from '@tinycld/core/ui/PlainInput'
import { ChevronDown } from 'lucide-react-native'
import { newRecordId } from 'pbtsdb/core'
import { useState } from 'react'
import { Pressable, Text } from 'react-native'

export interface RelationRecordPickerProps {
    target: string
    /**
     * The field to name a record by when the target's package declares no
     * `recordLabels` entry for it (the server's display-field heuristic).
     */
    displayField: string
    value: string
    onChange: (id: string) => void
    accessibilityLabel?: string
    /** Only records whose fields equal these values. */
    match?: Record<string, string | number | boolean>
    /** Offer to make a new record named from the search text. */
    create?: RelationCreate
}

// Relation targets are arbitrary collections declared by a package's
// automation catalog — not guaranteed to be registered in the client's
// pbtsdb store map (an unlinked package, or a name outside the tinycld
// schema entirely). collectionByName resolves it dynamically when it IS
// registered; the queries themselves no-op when it isn't, rather than
// conditionally skipping the useLiveQuery calls — hooks must run
// unconditionally.
//
// Raw useLiveQuery (not useMyLiveQuery) is correct here, same rationale as
// use-packages.ts: the collection may be a global, non-org-scoped store (or
// belong to another package entirely), so org/user scoping is the caller's
// concern, not this generic picker's — rows are already RLS-filtered by the
// server.

const SEARCH_DEBOUNCE_MS = 250

/**
 * The label the target's package declared, else the server's display field.
 * A declared label whose related collection is not registered here falls back
 * too: there would be no rows to read the related text from.
 */
function pickerLabel(target: string, displayField: string): RecordLabel {
    const declared = packageRecordLabels[target]
    if (!declared) return fieldLabel(displayField)
    if (declared.related && !collectionByName(declared.related.collection)) {
        return fieldLabel(displayField)
    }
    return declared
}

function pickerSources(
    target: string,
    displayField: string,
    match: RelationRecordPickerProps['match']
) {
    const label = pickerLabel(target, displayField)
    const sources: LabelSources = {
        collection: collectionByName(target),
        related: label.related ? collectionByName(label.related.collection) : undefined,
        match,
    }
    return { label, sources }
}

// Makes the record as the person through pbtsdb, so the target collection's
// own create rule and hooks apply (a new workbook gets its blank file there).
function useCreateRecord(
    target: string,
    create: RelationCreate | undefined,
    onCreated: (id: string) => void
) {
    const { user } = useAuth()
    return useMutation({
        mutationFn: function* (typed: string) {
            if (!create) return
            const suffix = create.nameSuffix ?? ''
            const name = typed.endsWith(suffix) ? typed : `${typed}${suffix}`
            const id = newRecordId()
            const owner = create.ownerField ? { [create.ownerField]: user.id } : {}
            const insert = insertByName(target, {
                ...create.values,
                ...owner,
                id,
                [create.nameField]: name,
            })
            if (!insert) return
            yield insert
            onCreated(id)
        },
    })
}

// Rendered only inside the open menu, so a closed picker runs no results query.
function PickerResults({
    target,
    displayField,
    value,
    onChange,
    match,
    create,
}: RelationRecordPickerProps) {
    const placeholderColor = useThemeColor('field-placeholder')
    const [search, setSearch] = useState('')
    // Debounced so each keystroke does not open a new subscription — the query's
    // identity is derived from the term it captures. Backslashes are dropped
    // because pbtsdb's escapeValue escapes `"` but not `\`, so a term containing
    // `\"` compiles to an unterminated filter literal and PocketBase answers 400.
    const term = search.trim().replace(/\\/g, '')
    const debouncedSearch = useDebouncedValue(term, SEARCH_DEBOUNCE_MS)
    const { label, sources } = pickerSources(target, displayField, match)
    const { isReady, records } = useLabelledRecords(sources, label, debouncedSearch)

    // The debounce window, plus the request it then issues, is a stretch of time
    // in which `records` still answers the PREVIOUS term. Rendering those rows is
    // not merely stale, it is wrong: the row a user reaches for is not the row
    // they typed for, so a click selects a different record — or, if the rows
    // shrank, lands on whatever is behind the menu. Show the searching hint until
    // the results answer the term that is actually in the box.
    const isSearching = debouncedSearch !== term || !isReady
    const matches = isSearching ? [] : records
    const createRecord = useCreateRecord(target, create, onChange)
    const canCreate = Boolean(create) && term !== '' && !createRecord.isPending

    return (
        <>
            <Menu.Custom className="px-2 pt-1 pb-2">
                <PlainInput
                    value={search}
                    onChangeText={setSearch}
                    placeholder="Search…"
                    placeholderTextColor={placeholderColor}
                    className="border rounded-lg px-2.5 py-1.5 text-sm text-foreground bg-background border-border"
                />
            </Menu.Custom>
            {matches.map(record => (
                <Menu.Item
                    key={record.id}
                    label={record.name}
                    isSelected={record.id === value}
                    onSelect={() => onChange(record.id)}
                />
            ))}
            <SearchingHint isVisible={isSearching} />
            <EmptyHint
                isVisible={!isSearching && matches.length === 0}
                hasSearch={Boolean(debouncedSearch)}
            />
            <MoreHint isVisible={!isSearching && matches.length >= VISIBLE_LIMIT} />
            <CreateItem
                isVisible={canCreate}
                label={`${create?.label ?? ''} “${term}”`}
                onSelect={() => createRecord.mutate(term)}
            />
        </>
    )
}

export function RelationRecordPicker(props: RelationRecordPickerProps) {
    const { target, displayField, value, onChange, accessibilityLabel } = props
    const mutedColor = useThemeColor('muted-foreground')
    const { label: recordLabel, sources } = pickerSources(target, displayField, props.match)
    const selectedRecord = useLabelledRecord(sources, recordLabel, value)

    if (!sources.collection) {
        return (
            <PlainInput
                value={value}
                onChangeText={onChange}
                accessibilityLabel={accessibilityLabel}
                placeholder={`record id — ${target} isn't installed here`}
                className="flex-1 border rounded-lg px-2.5 py-1.5 text-sm text-foreground bg-background border-border"
            />
        )
    }

    // The trigger names the selection from its own one-row query, so it stays
    // correct while the menu's window shows something else entirely. It falls
    // back to the raw id only while that row is still in flight.
    const label = selectedRecord?.name ?? (value || 'Select…')

    return (
        <Menu
            trigger={
                <Pressable
                    accessibilityLabel={accessibilityLabel}
                    className="flex-1 flex-row items-center justify-between border rounded-lg px-2.5 py-1.5 border-border bg-background"
                >
                    <Text className="text-sm text-foreground" numberOfLines={1}>
                        {label}
                    </Text>
                    <ChevronDown size={14} color={mutedColor} />
                </Pressable>
            }
            placement="bottom-start"
        >
            <PickerResults {...props} />
        </Menu>
    )
}

// Shown while the rows on hand still answer an earlier term. "No matches" would
// be a lie here — nothing has been asked yet — and the previous rows would be a
// worse one, because they are clickable.
function SearchingHint({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return <Text className="px-3 py-2 text-xs text-muted-foreground">Searching…</Text>
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

function CreateItem({
    isVisible,
    label,
    onSelect,
}: {
    isVisible: boolean
    label: string
    onSelect: () => void
}) {
    if (!isVisible) return null
    return <Menu.Item label={label} onSelect={onSelect} testID="relation-picker-create" />
}
