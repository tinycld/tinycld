import { Building2 } from 'lucide-react-native'
import { Text, View } from 'react-native'
import { PageHeader } from './console-ui'

// Single-org deployment: this process IS one org. Cross-org management (listing
// tenants, creating orgs + owners, impersonation) now lives in the multi-org
// router, which provisions each org as its own PocketBase process and will
// surface the operator's accessible orgs via a parent-domain cookie.
//
// TODO(de-org): render the accessible-org list from the parent-domain cookie the
// router sets (e.g. `tinycld_orgs=[{slug,name,url}]`) and link each row out to
// `https://<slug>.<domain>`. Until the router sets that cookie there is nothing
// org-scoped to show here, so this renders an explanatory empty state.
export function OrganizationsTab({ isVisible }: { isVisible: boolean; pb?: unknown }) {
    if (!isVisible) return null

    return (
        <View className="gap-6">
            <PageHeader
                title="Organizations"
                subtitle="Organizations are provisioned and managed by the multi-org router."
            />
            <View className="items-center gap-3 py-12">
                <Building2 size={32} className="text-muted-foreground" />
                <Text
                    className="text-center text-muted-foreground"
                    style={{ fontSize: 14, lineHeight: 20, maxWidth: 420 }}
                >
                    This deployment serves a single organization. Tenant provisioning and cross-org
                    switching are handled by the multi-org router.
                </Text>
            </View>
        </View>
    )
}
