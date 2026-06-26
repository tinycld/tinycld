import { trace } from '@tinycld/core/lib/debug-trace'
import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { useSortedPackages } from '@tinycld/core/lib/use-sorted-packages'
import { Redirect } from 'expo-router'

export default function OrgIndex() {
    const sorted = useSortedPackages()
    const orgHref = useOrgHref()
    const first = sorted[0]

    trace('OrgIndex render', {
        sortedCount: sorted.length,
        slugs: sorted.map(p => p.slug),
        firstSlug: first?.slug ?? null,
    })

    if (first) {
        const href = orgHref(first.slug as never)
        trace('OrgIndex redirect to package', { firstSlug: first.slug, href: JSON.stringify(href) })
        return <Redirect href={href} />
    }

    // No nav package to land on — a package-less shell, or an org whose only
    // contributors are settings-only (no nav entry). Redirect to settings rather
    // than rendering a dead-end skeleton forever, which left the org root with no
    // valid child and surfaced as "Unmatched Route" after login. On a build with
    // packages compiled in, useAccessiblePackages falls back to all of them while
    // the registry loads, so `sorted` is empty here only when there genuinely is
    // nothing to show — making the settings redirect safe (no transient flash).
    const settingsHref = orgHref('settings' as never)
    trace('OrgIndex redirect to settings (no packages)', { href: JSON.stringify(settingsHref) })
    return <Redirect href={settingsHref} />
}
