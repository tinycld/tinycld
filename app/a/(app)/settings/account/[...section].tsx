// Per-user package panels (manifest `accountSettings`), addressed under
// /settings/account/<pkgSlug>/<panelSlug>. No role gate: each panel acts only
// on the viewer's own data, and the server rules enforce that.
//
// A separate tree from settings/[...section].tsx so a package can use the same
// panel slug in both registries without one shadowing the other.

import { PackagePanelScreen } from '@tinycld/core/components/settings/PackagePanelScreen'
import { packageAccountSettings } from '@tinycld/core/lib/packages/derive-components'
import {
    declaresTerms,
    slugOfGroup,
    useTermsBlocked,
    useTermsOpen,
} from '@tinycld/core/lib/packages/use-package-terms'
import { useLocalSearchParams } from 'expo-router'

export default function PackageAccountSettingsSection() {
    // A package whose terms the owner has not accepted has no panels.
    const groups = useTermsOpen(packageAccountSettings, slugOfGroup)
    const { isReady: termsReady } = useTermsBlocked()
    const [pkgSlug] = useLocalSearchParams<{ section: string[] }>().section ?? []
    // Wait for the terms state: until then every package with terms is closed.
    // Only for such a package, so no other panel unmounts if the state is lost.
    if (declaresTerms(pkgSlug ?? '') && !termsReady) return null
    return <PackagePanelScreen groups={groups} />
}
