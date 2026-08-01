import { MenuActionItem } from '@tinycld/core/components/DropdownMenu'
import { OrgLogo } from '@tinycld/core/components/OrgLogo'
import { useAuth } from '@tinycld/core/lib/auth'
import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { navigateToOrgUrl } from '@tinycld/core/lib/org-url'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Menu, Separator } from '@tinycld/core/ui/menu'
import { useRouter } from 'expo-router'
import { Globe, LogOut, Settings, User } from 'lucide-react-native'
import { Pressable, Text, View } from 'react-native'
import { isCurrentOrg, type UserOrgEntry, useApexUrl, useUserOrgs } from './useUserOrgs'

export function UserMenu() {
    const railActiveText = useThemeColor('rail-active-text')
    const { user, logout } = useAuth()
    const orgHref = useOrgHref()
    const router = useRouter()
    const orgs = useUserOrgs()

    return (
        <Menu>
            <Menu.Trigger>
                <Pressable
                    className="size-8 rounded-full justify-center items-center"
                    style={{
                        backgroundColor: 'rgba(255,255,255,0.15)',
                    }}
                    accessibilityLabel="User menu"
                >
                    <User size={20} color={railActiveText} />
                </Pressable>
            </Menu.Trigger>
            <Menu.Portal>
                <Menu.Overlay />
                <Menu.Content presentation="popover" placement="top" align="start">
                    <View className="px-3 py-2">
                        <Text className="text-base font-bold text-foreground">{user.name}</Text>
                    </View>

                    <Separator />

                    <MenuActionItem
                        label="Settings"
                        icon={Settings}
                        onPress={() => router.push(orgHref('settings/personal'))}
                    />

                    <OrganizationsSection orgs={orgs} />

                    <Separator />

                    <MenuActionItem label="Sign out" icon={LogOut} onPress={logout} />
                </Menu.Content>
            </Menu.Portal>
        </Menu>
    )
}

// Cross-org switching: entries come from the parent-domain cookie the tenants
// write at login (useUserOrgs); each row is a full page load on the target
// org's own origin. Renders nothing on a standalone deployment (empty cookie).
// The cookie only knows orgs this browser has signed into, so the section ends
// with a link to the apex org-finder page — the discovery path for the rest.
function OrganizationsSection({ orgs }: { orgs: UserOrgEntry[] }) {
    const apexUrl = useApexUrl()
    if (orgs.length === 0) return null
    return (
        <>
            <Separator />
            <Menu.Label>Organizations</Menu.Label>
            {orgs.map(org => (
                <MenuActionItem
                    key={org.id}
                    label={org.name}
                    leading={<OrgLogo org={org} size={18} />}
                    isActive={isCurrentOrg(org)}
                    href={org.url}
                    onPress={() => navigateToOrgUrl(org.url)}
                />
            ))}
            {apexUrl !== null && (
                <MenuActionItem
                    label="Open another organization…"
                    icon={Globe}
                    href={apexUrl}
                    onPress={() => navigateToOrgUrl(apexUrl)}
                />
            )}
        </>
    )
}
