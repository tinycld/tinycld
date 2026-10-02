// Per-user package panels (manifest `accountSettings`), addressed under
// /settings/account/<pkgSlug>/<panelSlug>. No role gate: each panel acts only
// on the viewer's own data, and the server rules enforce that.
//
// A separate tree from settings/[...section].tsx so a package can use the same
// panel slug in both registries without one shadowing the other.

import { PackagePanelScreen } from '@tinycld/core/components/settings/PackagePanelScreen'
import { packageAccountSettings } from '@tinycld/core/lib/packages/derive-components'

export default function PackageAccountSettingsSection() {
    return <PackagePanelScreen groups={packageAccountSettings} />
}
