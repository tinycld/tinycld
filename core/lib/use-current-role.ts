import { eq } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { useAuth } from '@tinycld/core/lib/auth'
import { useStore } from '@tinycld/core/lib/pocketbase'

// Single-org deployment: a user's role lives on their `users` auth record, not on
// a membership junction. Read it directly for the current user.
export function useCurrentRole() {
    const { user } = useAuth()
    const [usersCollection] = useStore('users')

    const { data: rows } = useLiveQuery(
        query => query.from({ users: usersCollection }).where(({ users }) => eq(users.id, user.id)),
        [user.id]
    )

    const role = rows?.[0]?.role ?? null
    return {
        role,
        isOwner: role === 'owner',
        isAdmin: role === 'owner' || role === 'admin',
        isMember: role === 'member',
        isGuest: role === 'guest',
        canManageOrg: role === 'owner' || role === 'admin',
        canManageMembers: role === 'owner' || role === 'admin',
    }
}
