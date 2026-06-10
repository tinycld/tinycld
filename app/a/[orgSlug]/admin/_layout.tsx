import { DocumentTitle } from '@tinycld/core/components/DocumentTitle'
import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { useIsSuperAdmin } from '@tinycld/core/lib/use-is-super-admin'
import { Redirect, Slot } from 'expo-router'

// The in-shell Admin area. It renders inside WorkspaceLayout (rail + the
// AdminSidebar drives the section list) — unlike the standalone /admin route,
// which stays the superuser bootstrap/recovery entry point. Non-super-admins
// are bounced to the org home; the rail icon is already hidden from them, so
// this just guards a hand-typed URL.
export default function AdminLayout() {
    const isSuperAdmin = useIsSuperAdmin()
    const orgHref = useOrgHref()

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
