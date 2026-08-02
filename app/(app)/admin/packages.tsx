import { AdminScreen } from '@tinycld/core/components/setup/AdminScreen'
import { PackageManager } from '@tinycld/core/components/setup/PackageManager'
import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { pb } from '@tinycld/core/lib/pocketbase'
import { useCurrentRole } from '@tinycld/core/lib/use-current-role'
import { Redirect } from 'expo-router'

// Owner-only within the otherwise admin-wide console: installing, removing or
// version-changing a package rebuilds the artifact the whole deployment runs.
// AdminSidebar hides this section from admins and requireOwner rejects them
// server-side; this guards the hand-typed URL in between.
export default function AdminPackages() {
    const { isOwner, isReady } = useCurrentRole()
    const orgHref = useOrgHref()

    if (!isReady) return null
    if (!isOwner) return <Redirect href={orgHref('admin')} />

    return (
        <AdminScreen title="Packages">
            <PackageManager pb={pb} isVisible />
        </AdminScreen>
    )
}
