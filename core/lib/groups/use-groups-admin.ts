import { and, count, eq, not } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { mutation, performMutations, useMutation } from '@tinycld/core/lib/mutations'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { newRecordId } from 'pbtsdb/core'

export interface GroupInput {
    name: string
    description: string
}

/** Groups list with live member counts, plus create/update/delete. */
export function useGroupsAdmin() {
    const [groupsCollection, membersCollection] = useStore('groups', 'group_members')

    const { data: groupRows, isReady: groupsReady } = useLiveQuery(query =>
        query.from({ g: groupsCollection }).orderBy(({ g }) => g.name)
    )
    const { data: counts, isReady: countsReady } = useLiveQuery(query =>
        query
            .from({ m: membersCollection })
            .groupBy(({ m }) => m.group)
            .select(({ m }) => ({ group: m.group, n: count(m.id) }))
    )
    // Two queries joined in render rather than one .join(): a left join with
    // an aggregate is not expressible in one TanStack DB query today.
    const countByGroup = new Map((counts ?? []).map(c => [c.group, c.n]))
    const groups = (groupRows ?? []).map(g => ({
        id: g.id,
        name: g.name,
        description: g.description ?? '',
        memberCount: countByGroup.get(g.id) ?? 0,
    }))

    const update = useMutation<void, Error, { id: string; input: GroupInput }>({
        mutationFn: mutation(function* ({ id, input }) {
            yield groupsCollection.update(id, draft => {
                draft.name = input.name
                draft.description = input.description
            })
        }),
    })
    const remove = useMutation<void, Error, string>({
        mutationFn: mutation(function* (id) {
            yield groupsCollection.delete(id)
        }),
    })

    const createGroup = (input: GroupInput) =>
        performMutations(function* () {
            const id = newRecordId()
            yield groupsCollection.insert({ id, name: input.name, description: input.description })
            return id
        })

    return {
        groups,
        isReady: groupsReady && countsReady,
        createGroup,
        updateGroup: (id: string, input: GroupInput) => update.mutate({ id, input }),
        deleteGroup: (id: string) => remove.mutate(id),
        isPending: update.isPending || remove.isPending,
    }
}

/** Members of one group, the users who could still be added, and add/remove. */
export function useGroupMembersAdmin(groupId: string) {
    const [membersCollection, usersCollection] = useStore('group_members', 'users')

    const { data: memberRows } = useLiveQuery(
        query =>
            query
                .from({ m: membersCollection })
                .innerJoin({ u: usersCollection }, ({ m, u }) => eq(m.user, u.id))
                .where(({ m }) => eq(m.group, groupId))
                .select(({ m, u }) => ({
                    membershipId: m.id,
                    userId: u.id,
                    name: u.name,
                    email: u.email,
                })),
        [groupId]
    )
    const { data: eligible } = useLiveQuery(query =>
        query
            .from({ u: usersCollection })
            .where(({ u }) => and(not(eq(u.role, 'guest')), not(eq(u.disabled, true))))
            .orderBy(({ u }) => u.name)
            .select(({ u }) => ({ userId: u.id, name: u.name, email: u.email }))
    )

    const members = [...(memberRows ?? [])].sort((a, b) =>
        (a.name || a.email).localeCompare(b.name || b.email)
    )
    const memberIds = new Set(members.map(m => m.userId))
    const candidates = (eligible ?? []).filter(u => !memberIds.has(u.userId))

    const add = useMutation<void, Error, string>({
        mutationFn: mutation(function* (userId) {
            yield membersCollection.insert({ id: newRecordId(), group: groupId, user: userId })
        }),
    })
    const remove = useMutation<void, Error, string>({
        mutationFn: mutation(function* (membershipId) {
            yield membersCollection.delete(membershipId)
        }),
    })

    return {
        members,
        candidates,
        addMember: (userId: string) => add.mutate(userId),
        removeMember: (membershipId: string) => remove.mutate(membershipId),
        isPending: add.isPending || remove.isPending,
    }
}
