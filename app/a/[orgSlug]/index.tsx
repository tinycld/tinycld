import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { useSortedPackages } from '@tinycld/core/lib/use-sorted-packages'
import { Redirect } from 'expo-router'

export default function OrgIndex() {
    const sorted = useSortedPackages()
    const orgHref = useOrgHref()
    const first = sorted[0]

    if (first) {
        return <Redirect href={orgHref(first.slug as never)} />
    }

    // No nav package to land on — a package-less shell, or an org whose only
    // contributors are settings-only (no nav entry). Redirect to settings rather
    // than rendering a dead-end skeleton forever, which left the org root with no
    // valid child and surfaced as "Unmatched Route" after login. On a build with
    // packages compiled in, useAccessiblePackages falls back to all of them while
    // the registry loads, so `sorted` is empty here only when there genuinely is
    // nothing to show — making the settings redirect safe (no transient flash).
    return <Redirect href={orgHref('settings' as never)} />
}
