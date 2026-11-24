import { GuestEmptyState } from '@tinycld/core/components/GuestEmptyState'
import { trace } from '@tinycld/core/lib/debug-trace'
import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { hasUndecidedTerms } from '@tinycld/core/lib/packages/terms-state'
import { usePackageTermsList } from '@tinycld/core/lib/packages/use-package-terms'
import { useCurrentRole } from '@tinycld/core/lib/use-current-role'
import { useSortedPackagesResult } from '@tinycld/core/lib/use-sorted-packages'
import { Redirect } from 'expo-router'
import { usePackagePrefetch } from '~/lib/route-prefetch'

export default function OrgIndex() {
    const { packages: sorted, isReady } = useSortedPackagesResult()
    const { isGuest, isOwner } = useCurrentRole()
    const terms = usePackageTermsList()
    const orgHref = useOrgHref()
    const first = sorted[0]
    // Hold the redirect until the landing package's chunks are loaded, so its
    // screen and sidebar mount without a Suspense fallback (lib/route-prefetch.ts).
    const landingLoaded = usePackagePrefetch(isReady ? (first?.slug ?? 'settings') : null)

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
    if (!isReady || !landingLoaded || !terms.isReady) return null

    // An owner who has not decided on a package's terms is asked first: a
    // package added after setup, terms skipped in the wizard, or a new terms
    // version. A declined package does not ask again; Settings → Package terms
    // still lists it. Only this launch route asks: a deep link is not
    // interrupted, and the package's own route shows its terms.
    if (isOwner && hasUndecidedTerms(terms.pending)) {
        const termsHref = orgHref('settings/package-terms')
        trace('OrgIndex redirect to package terms', { href: JSON.stringify(termsHref) })
        return <Redirect href={termsHref} />
    }

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
