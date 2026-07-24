import { AdminScreen } from '@tinycld/core/components/setup/AdminScreen'
import { PackageManager } from '@tinycld/core/components/setup/PackageManager'
import { pb } from '@tinycld/core/lib/pocketbase'

export default function AdminPackages() {
    return (
        <AdminScreen title="Packages">
            <PackageManager pb={pb} isVisible />
        </AdminScreen>
    )
}
