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

// useSuperAdminStatus returns BOTH whether the current user is a super admin and
// whether the answer has settled (auth resolved + the super_admins live query
// loaded), so a redirect guard can wait for a definitive answer instead of acting
// on the transient initial `false`.
export function useSuperAdminStatus(): { isSuperAdmin: boolean; isReady: boolean } {
    const { user, isLoggedIn, isInitializing } = useAuth({ throwIfAnon: false })
    const [superAdminsCollection] = useStore('super_admins')

    const { data, isReady: queryReady } = useLiveQuery(
        query =>
            query
                .from({ super_admin: superAdminsCollection })
                .where(({ super_admin }) => eq(super_admin.user, user?.id ?? '')),
        [user?.id]
    )

    // Settled once auth is no longer initializing AND the live query has reached
    // its `ready` status (TanStack DB surfaces this as isReady). An anon user has
    // no query to wait on, so it's settled-and-not-admin as soon as auth resolves.
    const isReady = !isInitializing && (queryReady || !isLoggedIn)
    return { isSuperAdmin: isLoggedIn && (data?.length ?? 0) > 0, isReady }
}

export function useIsSuperAdmin(): boolean {
    return useSuperAdminStatus().isSuperAdmin
}
