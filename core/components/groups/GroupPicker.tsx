import { useLiveQuery } from '@tanstack/react-db'
import type { GroupRoleOption } from '@tinycld/core/lib/groups/types'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { Dialog } from '@tinycld/core/ui/dialog'
import { PlainInput } from '@tinycld/core/ui/PlainInput'
import { Search, UsersRound } from 'lucide-react-native'
import { useState } from 'react'
import { Pressable, Text, View } from 'react-native'

interface GroupPickerProps<Role extends string> {
    isVisible: boolean
    excludeIds: Set<string>
    /** Must be non-empty: the picker initialises its role selection from it. */
    roles: readonly GroupRoleOption<Role>[]
    onPick: (groupId: string, role: Role) => void
    onClose: () => void
}

interface Candidate {
    id: string
    name: string
    description: string
}

function useCandidates(query: string, excludeIds: Set<string>): Candidate[] {
    const [groupsCollection] = useStore('groups')
    const { data } = useLiveQuery(q => q.from({ g: groupsCollection }).orderBy(({ g }) => g.name))
    const needle = query.trim().toLowerCase()
    return (data ?? [])
        .filter(g => !excludeIds.has(g.id))
        .filter(g => !needle || g.name.toLowerCase().includes(needle))
        .map(g => ({ id: g.id, name: g.name, description: g.description }))
}

export function GroupPicker<Role extends string>(props: GroupPickerProps<Role>) {
    if (!props.isVisible) return null
    return <GroupPickerOpen {...props} />
}

function GroupPickerOpen<Role extends string>({
    excludeIds,
    roles,
    onPick,
    onClose,
}: GroupPickerProps<Role>) {
    const [query, setQuery] = useState('')
    const [role, setRole] = useState<Role>(roles[roles.length - 1].value)
    const candidates = useCandidates(query, excludeIds)

    const handlePick = (groupId: string) => {
        onPick(groupId, role)
        onClose()
    }

    return (
        <Dialog isOpen onClose={onClose} title="Add a group" size="md">
            <View className="px-5 pb-2 gap-3">
                <SearchRow query={query} onChange={setQuery} />
                <RolePicker roles={roles} role={role} onChange={setRole} />
            </View>
            <CandidateList
                candidates={candidates}
                hasQuery={query.trim().length > 0}
                onPick={handlePick}
            />
        </Dialog>
    )
}

function SearchRow({ query, onChange }: { query: string; onChange: (value: string) => void }) {
    const mutedColor = useThemeColor('muted')
    return (
        <View className="flex-row items-center gap-2 px-3 py-2 rounded-md border border-border bg-background">
            <Search size={15} color={mutedColor} strokeWidth={2.2} />
            <PlainInput
                testID="group-picker-search"
                value={query}
                onChangeText={onChange}
                placeholder="Search groups"
                placeholderTextColor={mutedColor}
                autoFocus
                className="flex-1 text-[13.5px] text-foreground"
            />
        </View>
    )
}

function RolePicker<Role extends string>({
    roles,
    role,
    onChange,
}: {
    roles: readonly GroupRoleOption<Role>[]
    role: Role
    onChange: (role: Role) => void
}) {
    return (
        <View className="flex-row flex-wrap gap-2">
            {roles.map(option => (
                <Pressable
                    key={option.value}
                    testID={`group-picker-role-${option.value}`}
                    accessibilityRole="button"
                    accessibilityState={{ selected: option.value === role }}
                    onPress={() => onChange(option.value)}
                    className={
                        option.value === role
                            ? 'px-3 py-1.5 rounded-md bg-primary'
                            : 'px-3 py-1.5 rounded-md border border-border bg-background'
                    }
                >
                    <Text
                        className={
                            option.value === role
                                ? 'text-[12px] font-semibold text-primary-foreground'
                                : 'text-[12px] font-semibold text-foreground'
                        }
                    >
                        {option.label}
                    </Text>
                </Pressable>
            ))}
        </View>
    )
}

function CandidateList({
    candidates,
    hasQuery,
    onPick,
}: {
    candidates: Candidate[]
    hasQuery: boolean
    onPick: (groupId: string) => void
}) {
    if (candidates.length === 0) {
        return (
            <View className="px-5 py-6">
                <Text className="text-[13px] text-muted">
                    {hasQuery ? 'No matching groups' : 'Every group is already added'}
                </Text>
            </View>
        )
    }
    return (
        <Dialog.Body contentClassName="pb-3">
            {candidates.map(candidate => (
                <CandidateRow key={candidate.id} candidate={candidate} onPick={onPick} />
            ))}
        </Dialog.Body>
    )
}

function CandidateRow({
    candidate,
    onPick,
}: {
    candidate: Candidate
    onPick: (groupId: string) => void
}) {
    const fgColor = useThemeColor('foreground')
    return (
        <View
            testID={`group-picker-row-${candidate.id}`}
            className="flex-row items-center gap-3 px-4 py-2.5"
        >
            <UsersRound size={20} color={fgColor} />
            <View className="flex-1 min-w-0">
                <Text className="text-[13.5px] font-medium text-foreground" numberOfLines={1}>
                    {candidate.name}
                </Text>
                <Description text={candidate.description} />
            </View>
            <Button size="sm" onPress={() => onPick(candidate.id)}>
                <ButtonText>Add</ButtonText>
            </Button>
        </View>
    )
}

function Description({ text }: { text: string }) {
    if (!text) return null
    return (
        <Text className="text-[12px] text-muted" numberOfLines={1}>
            {text}
        </Text>
    )
}
