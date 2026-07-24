import { AdminScreen } from '@tinycld/core/components/setup/AdminScreen'
import { OrganizationsTab } from '@tinycld/core/components/setup/OrganizationsTab'
import { pb } from '@tinycld/core/lib/pocketbase'

export default function AdminOrganizations() {
    return (
        <AdminScreen title="Organizations">
            <OrganizationsTab isVisible pb={pb} />
        </AdminScreen>
    )
}
