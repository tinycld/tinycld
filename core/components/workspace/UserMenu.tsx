import { MenuActionItem } from '@tinycld/core/components/DropdownMenu'
import { OrgLogo } from '@tinycld/core/components/OrgLogo'
import { useAuth } from '@tinycld/core/lib/auth'
import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { navigateToOrgUrl } from '@tinycld/core/lib/org-url'
import { canSwitchInPlace, isSameServer } from '@tinycld/core/lib/servers'
import { useToastStore } from '@tinycld/core/lib/stores/toast-store'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useSavedServers } from '@tinycld/core/lib/use-saved-servers'
import { Menu, Separator } from '@tinycld/core/ui/menu'
import { useRouter } from 'expo-router'
import { Globe, LogOut, Server, Settings, User } from 'lucide-react-native'
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

                    {/* The settings HUB, not the personal page: a generic
                        "Settings" label must reach every section. Rules, Members
                        and the rest are only listed on the hub, and on a tablet
                        (>=768dp, see the note below) this menu is the sole
                        Settings affordance — MoreDrawer never mounts. */}
                    <MenuActionItem
                        label="Settings"
                        icon={Settings}
                        onPress={() => router.push(orgHref('settings'))}
                    />

                    <OrganizationsSection orgs={orgs} />

                    <ServersSection hasOrgs={orgs.length > 0} />

                    <Separator />

                    <MenuActionItem label="Sign out" icon={LogOut} onPress={logout} />
                </Menu.Content>
            </Menu.Portal>
        </Menu>
    )
}

// The saved-server switcher, and the counterpart to OrganizationsSection below.
//
// This surface matters more than it looks: useBreakpoint() buckets on WIDTH ALONE,
// with no Platform check, so a native tablet at >=768dp renders PackageRail + this
// menu and NEVER mounts MoreDrawer. Without this section an iPad — the device where
// a work server and a home server are most plausible — would have no switcher
// outside Settings.
//
// `hasOrgs` keeps the two switchers from competing. On a router deployment the
// cookie gives orgs their real display names and an apex discovery link, which is
// strictly better than a hostname list, so Organizations wins the slot there. Where
// that cookie does not exist — a standalone deployment, or a browser that has only
// signed into one org — this fills the same slot instead. Never both: the same org
// would otherwise appear twice under two different names.
//
// Remove and Add are absent: Menu.Content closes on press, so there is nowhere to
// confirm a destructive action or show progress. Settings owns management; this is
// purely a switcher.
function ServersSection({ hasOrgs }: { hasOrgs: boolean }) {
    const { servers, activeOrigin, busyOrigin, canReload, switchTo } = useSavedServers()

    if (hasOrgs) return null
    // Nothing to switch between: on web the list always contains at least the
    // current origin, so a lone entry would just be a label for where you already
    // are — the org switcher's own `> 1` reasoning.
    if (servers.length < 2) return null

    // A menu item closes the popover on press, so unlike the drawer there is no
    // inline surface left to report a refused switch on — and switchToServer only
    // returns on FAILURE. Announce the outcome as a toast instead: ToastRenderer
    // sits in Providers at zIndex 10000, above the menu portal. In a dev build
    // every switch is refused, so say so up front rather than after the tap.
    //
    // Only meaningful where a switch happens in place. On web `switchTo` navigates
    // to the target origin, so there is no reload to be unavailable and the toast
    // would be both wrong and instantly discarded by the page load.
    function onSwitch(origin: string) {
        if (canSwitchInPlace() && !canReload) {
            useToastStore.getState().addToast({
                title: 'Restart to finish switching',
                body: 'This build cannot restart itself. Your choice is saved — reopen the app to land on it.',
                variant: 'warning',
                duration: 6000,
            })
        }
        switchTo(origin)
    }

    return (
        <>
            <Separator />
            <Menu.Label>Servers</Menu.Label>
            {servers.map(server => (
                <MenuActionItem
                    key={server.origin}
                    label={server.label}
                    icon={Server}
                    isActive={isSameServer(server.origin, activeOrigin ?? '')}
                    disabled={!!busyOrigin}
                    // A real href on web, matching the org switcher: the row IS a
                    // navigation there, so middle-click and open-in-new-tab should
                    // work. Native has no URL bar to hand it to.
                    href={canSwitchInPlace() ? undefined : server.origin}
                    onPress={() => onSwitch(server.origin)}
                />
            ))}
        </>
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
