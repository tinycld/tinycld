import { isSameServer, type SavedServer } from '@tinycld/core/lib/servers'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useSavedServers } from '@tinycld/core/lib/use-saved-servers'
import { Check, Plus, Server } from 'lucide-react-native'
import { ActivityIndicator, Pressable, StyleSheet, Text, View } from 'react-native'

// The saved-server switcher in the mobile More drawer.
//
// This fills the slot the "Organizations" section leaves empty on native:
// useUserOrgs() hard-returns [] off web, so that block never renders on a device.
// The two are mutually exclusive by platform, which makes them read as one
// "switch context" region wherever you are.
//
// Deliberately does NOT reuse ServerRow from settings/ServersSection: that one is
// styled for a settings card (foreground / muted-foreground / bg-surface-secondary)
// and would look wrong against the drawer's rail-background surface. The shared
// thing here is the hook, not the row.
//
// Remove is absent by design — it drops the auth blob and cancels the push
// registration, and removing the ACTIVE server restarts the JS context. That is
// the wrong outcome to sit one mis-tap away in a quick-switch list. Settings is
// the management surface.
export function ServersDrawerSection({
    onNavigate,
}: {
    // Runs an action and closes the drawer. Owned by MoreDrawer so this component
    // does not need to know the drawer's store.
    onNavigate: (action: () => void) => void
}) {
    const { servers, activeOrigin, busyOrigin, error, canReload, add, switchTo } = useSavedServers()
    const textColor = useThemeColor('rail-text')
    const activeColor = useThemeColor('rail-active-text')
    const borderColor = useThemeColor('border')

    // The hook self-gates by platform, so an empty list covers both web and the
    // (anomalous) no-saved-servers state.
    if (servers.length === 0) return null

    return (
        <>
            <View
                className="my-2 mx-3"
                style={{ height: StyleSheet.hairlineWidth, backgroundColor: borderColor }}
            />
            <Text
                className="text-xs font-semibold uppercase opacity-50 px-4 pt-1 pb-2"
                style={{ color: textColor }}
            >
                Servers
            </Text>

            <RestartNotice isVisible={!canReload} color={textColor} />

            {servers.map(server => (
                <ServerRow
                    key={server.origin}
                    server={server}
                    isActive={isSameServer(server.origin, activeOrigin ?? '')}
                    isBusy={busyOrigin === server.origin}
                    disabled={!!busyOrigin}
                    onSwitch={switchTo}
                    textColor={textColor}
                    activeColor={activeColor}
                />
            ))}

            <ErrorNotice message={error} />

            <Pressable
                className="flex-row items-center gap-3.5 px-4 py-3.5 rounded-lg"
                // Routed through onNavigate because add() pushes a route but does
                // NOT close the drawer, and the drawer renders inside the layout's
                // content region — so it would otherwise sit over the connect screen.
                onPress={() => onNavigate(add)}
                disabled={!!busyOrigin}
                accessibilityRole="button"
            >
                <Plus size={20} color={textColor} />
                <Text className="text-base font-medium" style={{ color: textColor }}>
                    Add server
                </Text>
            </Pressable>
        </>
    )
}

// In a dev build there is no mechanism to restart the JS context, so every switch
// is refused. Saying so up front turns the after-the-fact message into a
// confirmation instead of an apparent failure — and the switch is still worth
// attempting, since the active pointer IS persisted and a manual restart finishes it.
function RestartNotice({ isVisible, color }: { isVisible: boolean; color: string }) {
    if (!isVisible) return null
    return (
        <Text className="text-[11px] px-4 pb-2 opacity-60" style={{ color }}>
            Switching requires a restart in this build.
        </Text>
    )
}

function ErrorNotice({ message }: { message: string | null }) {
    if (!message) return null
    return (
        <View className="mx-3 my-1 rounded-lg p-3 bg-danger-soft">
            <Text className="text-xs text-danger">{message}</Text>
        </View>
    )
}

function ServerRow({
    server,
    isActive,
    isBusy,
    disabled,
    onSwitch,
    textColor,
    activeColor,
}: {
    server: SavedServer
    isActive: boolean
    isBusy: boolean
    disabled: boolean
    onSwitch: (origin: string) => void
    textColor: string
    activeColor: string
}) {
    const color = isActive ? activeColor : textColor

    return (
        <Pressable
            className="flex-row items-center gap-3.5 px-4 py-3.5 rounded-lg"
            // Unlike every other row in this drawer, a switch does NOT close it.
            // switchToServer only returns on FAILURE (success restarts the JS
            // context), so closing here would destroy the one surface the error
            // could appear on. The drawer vanishing IS the success signal.
            onPress={() => !isActive && onSwitch(server.origin)}
            disabled={isActive || disabled}
            accessibilityRole="button"
            accessibilityLabel={
                isActive ? `${server.label} (current server)` : `Switch to ${server.label}`
            }
        >
            <Server size={20} color={color} />
            <Text className="text-base font-medium flex-1" style={{ color }}>
                {server.label}
            </Text>
            <RowStatus isActive={isActive} isBusy={isBusy} color={activeColor} />
        </Pressable>
    )
}

function RowStatus({
    isActive,
    isBusy,
    color,
}: {
    isActive: boolean
    isBusy: boolean
    color: string
}) {
    if (isBusy) return <ActivityIndicator size="small" color={color} />
    if (!isActive) return null
    return <Check size={18} color={color} />
}
