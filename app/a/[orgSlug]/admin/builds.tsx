import { AdminScreen } from '@tinycld/core/components/setup/AdminScreen'
import { BuildHistoryTab } from '@tinycld/core/components/setup/BuildHistoryTab'
import { pb } from '@tinycld/core/lib/pocketbase'

export default function AdminBuilds() {
    return (
        <AdminScreen title="Build History">
            <BuildHistoryTab isVisible pb={pb} />
        </AdminScreen>
    )
}
