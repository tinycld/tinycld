import { isSameServer, isSavedServersSupported, type SavedServer } from '@tinycld/core/lib/servers'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useSavedServers } from '@tinycld/core/lib/use-saved-servers'
import { Check, Plus, Server, Trash2 } from 'lucide-react-native'
import { ActivityIndicator, Pressable, Text, View } from 'react-native'

// The saved-server list and switcher. Native-only: web is same-origin by design
// (resolveEnvAddress short-circuits to window.location.origin) and already has
// its own cookie-based org switcher, so this would be dead UI over an address
// that cannot change.
export function ServersSection() {
    const { servers, activeOrigin, busyOrigin, error, add, switchTo, remove } = useSavedServers()
    const fg = useThemeColor('foreground')

    if (!isSavedServersSupported()) return null

    return (
        <View className="gap-3">
            <Text className="text-xl font-bold text-foreground">Servers</Text>
            <View className="rounded-xl border border-border bg-surface-secondary p-4 gap-2">
                <Text className="text-[13px] text-muted-foreground">
                    Switch between the servers you've signed into. Each keeps its own account and
                    data — switching doesn't sign you out of the others.
                </Text>

                <View className="gap-1.5 mt-1">
                    {servers.map(server => (
                        <ServerRow
                            key={server.origin}
                            server={server}
                            isActive={isSameServer(server.origin, activeOrigin ?? '')}
                            isBusy={busyOrigin === server.origin}
                            disabled={!!busyOrigin}
                            onSwitch={switchTo}
                            onRemove={remove}
                        />
                    ))}
                </View>

                <ErrorNotice message={error} />

                <Pressable
                    onPress={add}
                    disabled={!!busyOrigin}
                    accessibilityRole="button"
                    className={`self-start rounded-lg mt-1 px-3 py-2 border border-border flex-row items-center gap-2 ${busyOrigin ? 'opacity-50' : 'opacity-100'}`}
                >
                    <Plus size={14} color={fg} />
                    <Text className="text-foreground font-semibold">Add server</Text>
                </Pressable>
            </View>
        </View>
    )
}

function ErrorNotice({ message }: { message: string | null }) {
    if (!message) return null
    return (
        <View className="rounded-lg p-3 bg-danger-soft">
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
    onRemove,
}: {
    server: SavedServer
    isActive: boolean
    isBusy: boolean
    disabled: boolean
    onSwitch: (origin: string) => void
    onRemove: (origin: string) => void
}) {
    const fg = useThemeColor('foreground')
    const muted = useThemeColor('muted-foreground')
    const danger = useThemeColor('danger')

    return (
        <View className="flex-row items-center gap-2 rounded-lg border border-border px-3 py-2.5">
            <Server size={15} color={muted} />
            <Pressable
                onPress={() => !isActive && onSwitch(server.origin)}
                disabled={isActive || disabled}
                accessibilityRole="button"
                accessibilityLabel={
                    isActive ? `${server.label} (current)` : `Switch to ${server.label}`
                }
                className="flex-1"
            >
                <Text className="text-foreground text-sm font-medium">{server.label}</Text>
                <Text className="text-[11px] text-muted-foreground">
                    {isActive ? 'Current server' : 'Tap to switch'}
                </Text>
            </Pressable>

            <RowStatus isActive={isActive} isBusy={isBusy} color={fg} />

            <Pressable
                onPress={() => onRemove(server.origin)}
                disabled={disabled}
                accessibilityRole="button"
                accessibilityLabel={`Remove ${server.label}`}
                className={`p-1.5 ${disabled ? 'opacity-40' : 'opacity-100'}`}
            >
                <Trash2 size={15} color={danger} />
            </Pressable>
        </View>
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
    if (isBusy) return <ActivityIndicator size="small" />
    if (!isActive) return null
    return <Check size={16} color={color} />
}
