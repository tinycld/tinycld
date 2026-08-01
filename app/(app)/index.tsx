import { GuestEmptyState } from '@tinycld/core/components/GuestEmptyState'
import { trace } from '@tinycld/core/lib/debug-trace'
import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { useCurrentRole } from '@tinycld/core/lib/use-current-role'
import { useSortedPackagesResult } from '@tinycld/core/lib/use-sorted-packages'
import { Redirect } from 'expo-router'

export default function OrgIndex() {
    const { packages: sorted, isReady } = useSortedPackagesResult()
    const { isGuest } = useCurrentRole()
    const orgHref = useOrgHref()
    const first = sorted[0]

    trace('OrgIndex render', {
        sortedCount: sorted.length,
        slugs: sorted.map(p => p.slug),
        firstSlug: first?.slug ?? null,
        isReady,
    })

    // Access data now default-denies while loading, so an empty list can mean
    // "still loading" rather than "genuinely nothing". Wait for isReady before
    // deciding where to land — redirecting to settings prematurely would flash
    // then bounce to the real package once the registry/overrides arrive.
    if (!isReady) return null

    if (first) {
        const href = orgHref(first.slug as never)
        trace('OrgIndex redirect to package', { firstSlug: first.slug, href: JSON.stringify(href) })
        return <Redirect href={href} />
    }

    // A guest with zero accessible packages is the expected state for a
    // share-link visitor, not a broken assembly: explain it instead of
    // silently dumping them into Settings with no way to understand why the
    // app looks empty.
    if (isGuest) {
        trace('OrgIndex guest empty state')
        return <GuestEmptyState />
    }

    // No nav package to land on — a package-less shell, or an org whose only
    // contributors are settings-only (no nav entry). Redirect to settings rather
    // than rendering a dead-end skeleton forever, which left the org root with no
    // valid child and surfaced as "Unmatched Route" after login. isReady above
    // guarantees `sorted` is empty here only when there genuinely is nothing to
    // show — making the settings redirect safe (no transient flash).
    const settingsHref = orgHref('settings' as never)
    trace('OrgIndex redirect to settings (no packages)', { href: JSON.stringify(settingsHref) })
    return <Redirect href={settingsHref} />
}
