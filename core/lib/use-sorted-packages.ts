import { useAccessiblePackagesResult } from '@tinycld/core/lib/use-accessible-packages'
import { useUserPreference } from '@tinycld/core/lib/use-user-preference'
import { useMemo } from 'react'

// Returns the nav packages in display order PLUS whether the underlying access
// data has settled. Callers that redirect on an empty list (org-index) must
// wait for isReady so they don't act on the transient default-deny empty state.
export function useSortedPackagesResult() {
    const { packages, isReady } = useAccessiblePackagesResult()
    const [pkgOrder] = useUserPreference('core', 'pkg_order', [] as string[])

    const sorted = useMemo(() => {
        // Packages without a nav entry (e.g. settings-only contributors like
        // google-takeout-import) must not appear in the rail.
        const navPackages = packages.filter(p => p.nav)
        if (!pkgOrder.length) {
            return [...navPackages].sort((a, b) => (a.nav?.order ?? 99) - (b.nav?.order ?? 99))
        }
        const orderMap = new Map(pkgOrder.map((slug, i) => [slug, i]))
        return [...navPackages].sort((a, b) => {
            const aIdx = orderMap.get(a.slug) ?? 999 + (a.nav?.order ?? 99)
            const bIdx = orderMap.get(b.slug) ?? 999 + (b.nav?.order ?? 99)
            return aIdx - bIdx
        })
    }, [packages, pkgOrder])

    return { packages: sorted, isReady }
}

export function useSortedPackages() {
    return useSortedPackagesResult().packages
}
