import { eq } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { Text, View } from 'react-native'

/**
 * Read-only: the groups one user belongs to. Membership is edited on
 * Settings → Groups so there is one place that changes it.
 */
export function MemberGroupChips({ userId }: { userId: string }) {
    const [membersCollection, groupsCollection] = useStore('group_members', 'groups')
    const { data: rows } = useLiveQuery(
        query =>
            query
                .from({ m: membersCollection })
                .innerJoin({ g: groupsCollection }, ({ m, g }) => eq(m.group, g.id))
                .where(({ m }) => eq(m.user, userId))
                .select(({ g }) => ({ id: g.id, name: g.name })),
        [userId]
    )
    // .orderBy() after .innerJoin() isn't supported by this TanStack DB
    // version (see useGroupMembersAdmin for the same pattern), so sort here.
    const groups = [...(rows ?? [])].sort((a, b) => a.name.localeCompare(b.name))
    if (groups.length === 0) return null
    return (
        <View className="gap-2">
            <Text className="text-[12px] font-semibold text-muted uppercase tracking-wide">
                Groups
            </Text>
            <View className="flex-row flex-wrap gap-1.5">
                {groups.map(group => (
                    <View
                        key={group.id}
                        testID={`member-group-chip-${group.name}`}
                        className="px-2.5 py-1 rounded-full bg-foreground/[0.06]"
                    >
                        <Text className="text-[12px] font-medium text-foreground">
                            {group.name}
                        </Text>
                    </View>
                ))}
            </View>
        </View>
    )
}
