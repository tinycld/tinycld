import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { useCurrentRole } from '@tinycld/core/lib/use-current-role'
import { Redirect } from 'expo-router'

// Admin landing → Packages for an owner, the most-used section (matches
// SetupDashboard's default tab). Admins can't open Packages, so they land on
// Organizations instead — routing them to Packages would redirect straight
// back here and loop. The AdminSidebar exposes every section they can reach.
export default function AdminIndex() {
    const orgHref = useOrgHref()
    const { isOwner, isReady } = useCurrentRole()

    if (!isReady) return null

    return <Redirect href={orgHref(isOwner ? 'admin/packages' : 'admin/organizations')} />
}
