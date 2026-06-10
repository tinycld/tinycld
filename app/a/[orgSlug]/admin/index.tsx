import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { Redirect } from 'expo-router'

// Admin landing → Packages, the most-used section (matches SetupDashboard's
// defaultTab). The AdminSidebar exposes every section.
export default function AdminIndex() {
    const orgHref = useOrgHref()
    return <Redirect href={orgHref('admin/packages')} />
}
