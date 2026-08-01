import { AdminScreen } from '@tinycld/core/components/setup/AdminScreen'
import { OrganizationsTab } from '@tinycld/core/components/setup/OrganizationsTab'

// Single-org: the tab is a static explainer pointing at the multi-org router,
// which owns tenant provisioning. Kept (rather than removed with the create
// form) so an operator looking for "where did organizations go?" finds an
// answer instead of a missing nav entry.
export default function AdminOrganizations() {
    return (
        <AdminScreen title="Organizations">
            <OrganizationsTab isVisible />
        </AdminScreen>
    )
}
