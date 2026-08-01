import { AdminScreen } from '@tinycld/core/components/setup/AdminScreen'
import { SuperAdminsTab } from '@tinycld/core/components/setup/SuperAdminsTab'
import { pb } from '@tinycld/core/lib/pocketbase'

export default function AdminSuperAdmins() {
    return (
        <AdminScreen title="Super Admins">
            <SuperAdminsTab isVisible pb={pb} />
        </AdminScreen>
    )
}
