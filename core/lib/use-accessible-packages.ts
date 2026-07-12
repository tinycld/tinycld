import { eq } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { usePackages } from '@tinycld/core/lib/packages/use-packages'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useCurrentRole } from '@tinycld/core/lib/use-current-role'
import { useOrgLiveQuery } from '@tinycld/core/lib/use-org-live-query'

type AccessiblePackage = ReturnType<typeof usePackages>[number]

export interface AccessiblePackagesResult {
    packages: AccessiblePackage[]
    // False until the registry + per-user override queries have all loaded.
    // Callers that must decide "there is genuinely nothing to show" (e.g. the
    // org-index settings redirect) should wait for this rather than acting on
    // the transient empty list during the default-deny load window.
    isReady: boolean
}

export function useAccessiblePackagesResult(): AccessiblePackagesResult {
    const packages = usePackages()
    const { role, userOrgId } = useCurrentRole()
    const [orgPkgAccessCollection] = useStore('org_pkg_access')
    const [pkgRegistryCollection] = useStore('pkg_registry')
    const [orgPkgEnabledCollection] = useStore('org_pkg_enabled')

    // Global registry: which packages are active (bundled or installed)
    const { data: registryRecords, isReady: registryReady } = useLiveQuery(
        query =>
            query
                .from({ pkg_registry: pkgRegistryCollection })
                .where(({ pkg_registry }) => eq(pkg_registry.status, 'bundled')),
        []
    )

    // Also include 'installed' status packages
    const { data: installedRecords, isReady: installedReady } = useLiveQuery(
        query =>
            query
                .from({ pkg_registry: pkgRegistryCollection })
                .where(({ pkg_registry }) => eq(pkg_registry.status, 'installed')),
        []
    )

    // Org-level package toggles
    const { data: orgToggles } = useOrgLiveQuery(
        (query, { orgId }) =>
            query
                .from({ org_pkg_enabled: orgPkgEnabledCollection })
                .where(({ org_pkg_enabled }) => eq(org_pkg_enabled.org, orgId)),
        []
    )

    // User-level access overrides
    const { data: overrides, isReady: overridesReady } = useOrgLiveQuery(
        query =>
            query
                .from({ org_pkg_access: orgPkgAccessCollection })
                .where(({ org_pkg_access }) => eq(org_pkg_access.user_org, userOrgId)),
        [userOrgId]
    )

    // Build set of globally active package slugs from registry
    const allActiveRecords = [...(registryRecords ?? []), ...(installedRecords ?? [])]
    const activeSlugs = new Set(allActiveRecords.map(r => r.slug))

    // Build map of org-level disabled packages
    const orgDisabledSlugs = new Set((orgToggles ?? []).filter(t => !t.enabled).map(t => t.pkg))

    // Start with packages that are both compiled-in and active in registry.
    // Only fall back to "all compiled-in packages" once the registry queries
    // have actually LOADED and are legitimately empty — NOT while they're still
    // loading (empty-on-first-load), which would flash every package before the
    // registry arrives. Distinguish loaded-and-empty from not-yet-loaded via
    // isReady rather than the record count.
    const registryLoaded = registryReady && installedReady
    const hasRegistry = allActiveRecords.length > 0
    let filtered =
        registryLoaded && !hasRegistry
            ? packages
            : packages.filter(pkg => activeSlugs.has(pkg.slug))

    // Remove org-level disabled packages
    filtered = filtered.filter(pkg => !orgDisabledSlugs.has(pkg.slug))

    // Admins/owners see everything active; they have no per-user overrides to
    // wait on, so they're ready as soon as the registry has loaded.
    if (role === 'owner' || role === 'admin') {
        return { packages: filtered, isReady: registryLoaded }
    }

    // Non-admins are gated on BOTH the registry AND their per-user overrides.
    // Default-DENY until overrides load: an empty overrideMap during the load
    // window would read as "no restrictions" and flash forbidden packages for a
    // restricted member/guest, so show nothing until overrides are ready.
    const isReady = registryLoaded && overridesReady
    if (!isReady) return { packages: [], isReady: false }

    const overrideMap = new Map(
        (overrides ?? []).map(o => [o.pkg, o.access as 'full' | 'readonly' | 'none'])
    )

    const accessible = filtered.filter(pkg => {
        const access = overrideMap.get(pkg.slug)
        if (role === 'guest') return access === 'full' || access === 'readonly'
        return access !== 'none'
    })
    return { packages: accessible, isReady: true }
}

// Backward-compatible array accessor: the accessible package list only.
export function useAccessiblePackages(): AccessiblePackage[] {
    return useAccessiblePackagesResult().packages
}
