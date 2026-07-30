import { and, eq } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { useAuth } from '@tinycld/core/lib/auth'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useCurrentRole } from '@tinycld/core/lib/use-current-role'

export type PackageAccessLevel = 'full' | 'readonly' | 'none'

export interface PkgAccessResult {
    access: PackageAccessLevel
    // False until the caller's override row has loaded. `access` is 'none'
    // during that window (default-deny); callers that show a "no access"
    // surface must wait for isReady or they flash it at every member.
    isReady: boolean
}

export function usePkgAccessResult(pkgSlug: string): PkgAccessResult {
    const { role } = useCurrentRole()
    const { user } = useAuth()
    const userId = user.id
    const [orgPkgAccessCollection] = useStore('org_pkg_access')

    const { data: overrides, isReady } = useLiveQuery(
        query =>
            query
                .from({ org_pkg_access: orgPkgAccessCollection })
                .where(({ org_pkg_access }) =>
                    and(eq(org_pkg_access.user, userId), eq(org_pkg_access.pkg, pkgSlug))
                ),
        [userId, pkgSlug]
    )

    // Owners/admins always have full access — no override row to wait for.
    if (role === 'owner' || role === 'admin') return { access: 'full', isReady: true }

    // Default-DENY until the override query has loaded: a member with a
    // readonly/none override otherwise reads 'full' during the load window and
    // flashes forbidden UI. Once loaded we apply the real default (member: full
    // unless overridden; guest: none unless overridden). The 'none' floor here
    // is transient — it resolves as soon as the org_pkg_access query is ready.
    if (!isReady) return { access: 'none', isReady: false }

    const override = overrides?.[0]

    if (role === 'guest') {
        return { access: (override?.access as PackageAccessLevel) ?? 'none', isReady: true }
    }

    // member: default full, override to restrict
    return { access: (override?.access as PackageAccessLevel) ?? 'full', isReady: true }
}

export function usePkgAccess(pkgSlug: string): PackageAccessLevel {
    return usePkgAccessResult(pkgSlug).access
}
