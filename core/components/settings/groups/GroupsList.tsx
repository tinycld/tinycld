import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { ChevronRight, UsersRound } from 'lucide-react-native'
import { Pressable, Text, View } from 'react-native'

export interface GroupListRow {
    id: string
    name: string
    description: string
    memberCount: number
}

interface GroupsListProps {
    groups: GroupListRow[]
    isReady: boolean
    onOpen: (groupId: string) => void
}

function memberLabel(count: number) {
    return count === 1 ? '1 member' : `${count} members`
}

export function GroupsList({ groups, isReady, onOpen }: GroupsListProps) {
    const mutedColor = useThemeColor('muted-foreground')
    if (isReady && groups.length === 0) return <EmptyState color={mutedColor} />
    return (
        <View className="rounded-xl overflow-hidden bg-surface-secondary border border-border">
            {groups.map((group, index) => (
                <GroupRow key={group.id} group={group} isFirst={index === 0} onOpen={onOpen} />
            ))}
        </View>
    )
}

function GroupRow({
    group,
    isFirst,
    onOpen,
}: {
    group: GroupListRow
    isFirst: boolean
    onOpen: (id: string) => void
}) {
    const mutedColor = useThemeColor('muted-foreground')
    const fgColor = useThemeColor('foreground')
    return (
        <Pressable
            testID={`group-row-${group.name}`}
            onPress={() => onOpen(group.id)}
            className="flex-row items-center gap-3 border-border"
            style={{ paddingVertical: 12, paddingHorizontal: 14, borderTopWidth: isFirst ? 0 : 1 }}
        >
            <UsersRound size={20} color={fgColor} />
            <View className="flex-1 min-w-0">
                <Text
                    className="text-foreground"
                    style={{ fontSize: 14, fontWeight: '600' }}
                    numberOfLines={1}
                >
                    {group.name}
                </Text>
                <Text className="text-muted-foreground" style={{ fontSize: 12 }} numberOfLines={1}>
                    {memberLabel(group.memberCount)}
                    {group.description ? ` · ${group.description}` : ''}
                </Text>
            </View>
            <ChevronRight size={15} color={mutedColor} />
        </Pressable>
    )
}

function EmptyState({ color }: { color: string }) {
    return (
        <View
            className="items-center gap-3 rounded-xl bg-surface-secondary border border-border"
            style={{ paddingVertical: 32, paddingHorizontal: 24 }}
        >
            <UsersRound size={28} color={color} />
            <Text className="text-foreground" style={{ fontSize: 15, fontWeight: '600' }}>
                No groups yet
            </Text>
            <Text className="text-muted-foreground" style={{ fontSize: 13, textAlign: 'center' }}>
                Create a group to share boards, files and calendars with a whole team at once.
            </Text>
        </View>
    )
}
