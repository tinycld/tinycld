import { DocumentTitle } from '@tinycld/core/components/DocumentTitle'
import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { useSuperAdminStatus } from '@tinycld/core/lib/use-is-super-admin'
import { Redirect, Slot } from 'expo-router'

// The in-shell Admin area. It renders inside WorkspaceLayout (rail + the
// AdminSidebar drives the section list) — unlike the standalone /admin route,
// which stays the superuser bootstrap/recovery entry point. Non-super-admins
// are bounced to the org home; the rail icon is already hidden from them, so
// this just guards a hand-typed URL.
export default function AdminLayout() {
    const { isSuperAdmin, isReady } = useSuperAdminStatus()
    const orgHref = useOrgHref()

    // Wait for the answer to settle before redirecting — acting on the transient
    // initial `false` would bounce a legitimate super admin who deep-links here on
    // a cold load (the previous safety relied implicitly on auth-store preload
    // ordering). Render nothing until then.
    if (!isReady) {
        return null
    }
    if (!isSuperAdmin) {
        return <Redirect href={orgHref('')} />
    }

    return (
        <>
            <DocumentTitle pkg="Admin" />
            <Slot />
        </>
    )
}
