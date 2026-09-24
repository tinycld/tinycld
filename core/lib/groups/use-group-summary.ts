import { count, eq } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { useStore } from '@tinycld/core/lib/pocketbase'

/** One group's display data plus its live member count, for a grant row. */
export function useGroupSummary(groupId: string) {
    const [groupsCollection, membersCollection] = useStore('groups', 'group_members')

    const { data: groups, isReady: groupReady } = useLiveQuery({
        query: query => query.from({ g: groupsCollection }).where(({ g }) => eq(g.id, groupId)),
    })
    const { data: counts, isReady: countReady } = useLiveQuery({
        query: query =>
            query
                .from({ m: membersCollection })
                .where(({ m }) => eq(m.group, groupId))
                .groupBy(({ m }) => m.group)
                .select(({ m }) => ({ group: m.group, n: count(m.id) })),
    })

    const group = groups?.[0]
    return {
        name: group?.name ?? '',
        description: group?.description ?? '',
        memberCount: counts?.[0]?.n ?? 0,
        isReady: groupReady && countReady,
    }
}
