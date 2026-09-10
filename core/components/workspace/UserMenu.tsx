import { useAuth } from '@tinycld/core/lib/auth'
import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { canSwitchInPlace, isSameServer } from '@tinycld/core/lib/servers'
import { useToastStore } from '@tinycld/core/lib/stores/toast-store'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useSavedServers } from '@tinycld/core/lib/use-saved-servers'
import { Menu } from '@tinycld/core/ui/menu'
import { useRouter } from 'expo-router'
import { LogOut, Server, Settings, User } from 'lucide-react-native'
import { Pressable, Text } from 'react-native'

export function UserMenu() {
    const railActiveText = useThemeColor('rail-active-text')
    const { user, logout } = useAuth()
    const orgHref = useOrgHref()
    const router = useRouter()

    return (
        <Menu
            trigger={
                <Pressable
                    className="size-8 rounded-full justify-center items-center"
                    style={{
                        backgroundColor: 'rgba(255,255,255,0.15)',
                    }}
                    accessibilityLabel="User menu"
                >
                    <User size={20} color={railActiveText} />
                </Pressable>
            }
            placement="top-start"
            title={user.name}
        >
            <Menu.Custom className="px-3 py-2">
                <Text className="text-base font-bold text-foreground">{user.name}</Text>
            </Menu.Custom>

            <Menu.Separator />

            {/* The settings HUB, not the personal page: a generic
                "Settings" label must reach every section. Rules, Members
                and the rest are only listed on the hub, and on a tablet
                (>=768dp, see the note below) this menu is the sole
                Settings affordance — MoreDrawer never mounts. */}
            <Menu.Item
                label="Settings"
                icon={Settings}
                onSelect={() => router.push(orgHref('settings'))}
            />

            <ServersSection />

            <Menu.Separator />

            <Menu.Item label="Sign out" icon={LogOut} onSelect={logout} />
        </Menu>
    )
}

// The saved-server switcher.
//
// This surface matters more than it looks: useBreakpoint() buckets on WIDTH ALONE,
// with no Platform check, so a native tablet at >=768dp renders PackageRail + this
// menu and NEVER mounts MoreDrawer. Without this section an iPad — the device where
// a work server and a home server are most plausible — would have no switcher
// outside Settings.
//
// Remove and Add are absent: a menu row closes the menu when chosen, so there is nowhere to
// confirm a destructive action or show progress. Settings owns management; this is
// purely a switcher.
function ServersSection() {
    const { servers, activeOrigin, busyOrigin, canReload, switchTo } = useSavedServers()

    // Nothing to switch between: on web the list always contains at least the
    // current origin, so a lone entry would just be a label for where you already
    // are.
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
            <Menu.Separator />
            <Menu.Section label="Servers">
                {servers.map(server => (
                    <Menu.Item
                        key={server.origin}
                        label={server.label}
                        icon={Server}
                        isSelected={isSameServer(server.origin, activeOrigin ?? '')}
                        isDisabled={!!busyOrigin}
                        // A real href on web: the row IS a navigation there, so
                        // middle-click and open-in-new-tab should work. Native has
                        // no URL bar to hand it to.
                        href={canSwitchInPlace() ? undefined : server.origin}
                        onSelect={() => onSwitch(server.origin)}
                    />
                ))}
            </Menu.Section>
        </>
    )
}
