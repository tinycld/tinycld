import { OrgLogo } from '@tinycld/core/components/OrgLogo'
import { navigateToOrgUrl } from '@tinycld/core/lib/org-url'
import { Building2, ExternalLink } from 'lucide-react-native'
import { Pressable, Text, View } from 'react-native'
import { isCurrentOrg, type UserOrgEntry, useUserOrgs } from '../workspace/useUserOrgs'
import { PageHeader } from './console-ui'

// Single-org deployment: this process IS one org. Cross-org management
// (provisioning tenants, creating orgs + owners) lives in the multi-org
// router's control plane. What this tab CAN show is the switcher list: the
// orgs this browser has signed into, from the parent-domain cookie each
// router-managed tenant upserts at login (lib/org-cookie.ts). Each row links
// out to that org's own origin.
export function OrganizationsTab({ isVisible }: { isVisible: boolean }) {
    const orgs = useUserOrgs()
    if (!isVisible) return null

    return (
        <View className="gap-6">
            <PageHeader
                title="Organizations"
                subtitle="Organizations are provisioned and managed by the multi-org router."
            />
            <OrgList orgs={orgs} />
            <EmptyState isVisible={orgs.length === 0} />
        </View>
    )
}

function OrgList({ orgs }: { orgs: UserOrgEntry[] }) {
    if (orgs.length === 0) return null
    return (
        <View className="gap-2">
            {orgs.map(org => (
                <OrgRow key={org.id} org={org} />
            ))}
        </View>
    )
}

function OrgRow({ org }: { org: UserOrgEntry }) {
    const active = isCurrentOrg(org)
    return (
        <Pressable
            testID={`org-row-${org.slug}`}
            disabled={active}
            onPress={() => navigateToOrgUrl(org.url)}
            className={`flex-row items-center gap-3 rounded-lg border p-3 ${
                active ? 'border-primary' : 'border-border'
            }`}
        >
            <OrgLogo org={org} size={28} />
            <View className="flex-1">
                <Text className="text-foreground font-semibold">{org.name}</Text>
                <Text className="text-muted-foreground text-[13px]">{org.url}</Text>
            </View>
            {active ? (
                <Text className="text-muted-foreground text-[13px]">Current</Text>
            ) : (
                <ExternalLink size={16} className="text-muted-foreground" />
            )}
        </Pressable>
    )
}

function EmptyState({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return (
        <View className="items-center gap-3 py-12">
            <Building2 size={32} className="text-muted-foreground" />
            <Text
                className="text-center text-muted-foreground"
                style={{ fontSize: 14, lineHeight: 20, maxWidth: 420 }}
            >
                This deployment serves a single organization. Organizations you sign into on other
                subdomains of the same deployment will appear here for quick switching.
            </Text>
        </View>
    )
}
