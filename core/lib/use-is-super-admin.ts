import { eq } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { useAuth } from '@tinycld/core/lib/auth'
import { useStore } from '@tinycld/core/lib/pocketbase'

/**
 * Reactive source of truth for super-admin status: live-queries the
 * super_admins junction for a row matching the current user. The collection's
 * RLS lets a caller read only their OWN row, so this returns true exactly when
 * the signed-in user is a super admin. Reflects grants/revokes without a
 * re-login (the store is preloaded and subscribed). Returns false when anon.
 */
export function useIsSuperAdmin(): boolean {
    const { user, isLoggedIn } = useAuth({ throwIfAnon: false })
    const [superAdminsCollection] = useStore('super_admins')

    const { data } = useLiveQuery(
        query =>
            query
                .from({ super_admin: superAdminsCollection })
                .where(({ super_admin }) => eq(super_admin.user, user?.id ?? '')),
        [user?.id]
    )

    return isLoggedIn && (data?.length ?? 0) > 0
}
