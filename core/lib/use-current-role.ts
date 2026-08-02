import { eq } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { useAuth } from '@tinycld/core/lib/auth'
import { useStore } from '@tinycld/core/lib/pocketbase'

// Single-org deployment: a user's role lives on their `users` auth record, not on
// a membership junction. Read it directly for the current user.
//
// Anon-tolerant (throwIfAnon: false) because role is now the ONLY privilege
// axis, so pre-auth surfaces — the /setup bootstrap door, the nav rail during
// a cold load — ask about it before there is a session. An anon caller gets
// role=null with isReady true: settled, and not privileged.
export function useCurrentRole() {
    const { user, isLoggedIn, isInitializing } = useAuth({ throwIfAnon: false })
    const [usersCollection] = useStore('users')

    const { data: rows, isReady: queryReady } = useLiveQuery(
        query =>
            query
                .from({ users: usersCollection })
                .where(({ users }) => eq(users.id, user?.id ?? '')),
        [user?.id]
    )

    const role = rows?.[0]?.role ?? null
    return {
        role,
        // Whether the answer has SETTLED. `role` is null both while the query
        // loads and when the user genuinely has no role, so a redirect guard
        // that acts on the transient null bounces a legitimate admin who
        // deep-links on a cold load. Gate on this before redirecting. An anon
        // user has no query to wait on — settled as soon as auth resolves.
        isReady: !isInitializing && (queryReady || !isLoggedIn),
        isOwner: role === 'owner',
        isAdmin: role === 'owner' || role === 'admin',
        isMember: role === 'member',
        isGuest: role === 'guest',
        canManageOrg: role === 'owner' || role === 'admin',
        canManageMembers: role === 'owner' || role === 'admin',
    }
}
