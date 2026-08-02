import { DocumentTitle } from '@tinycld/core/components/DocumentTitle'
import { SkeletonLayout } from '@tinycld/core/components/workspace/SkeletonLayout'
import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { useCurrentRole } from '@tinycld/core/lib/use-current-role'
import { Redirect, Slot } from 'expo-router'

// The in-shell Admin area. It renders inside WorkspaceLayout (rail + the
// AdminSidebar drives the section list) — unlike the standalone /admin route,
// which stays the superuser bootstrap/recovery entry point. Owners and admins
// both reach it; package management inside is owner-only (AdminSidebar hides
// that section and requireOwner enforces it server-side). Members and guests
// are bounced to the org home; the rail icon is already hidden from them, so
// this just guards a hand-typed URL.
export default function AdminLayout() {
    const { isAdmin, isReady } = useCurrentRole()
    const orgHref = useOrgHref()

    // Wait for the answer to settle before redirecting — acting on the transient
    // initial `false` would bounce a legitimate admin who deep-links here on a
    // cold load. Show the skeleton until then, matching the parent/sibling guards.
    if (!isReady) {
        return <SkeletonLayout />
    }
    if (!isAdmin) {
        return <Redirect href={orgHref('')} />
    }

    return (
        <>
            <DocumentTitle pkg="Admin" />
            <Slot />
        </>
    )
}
