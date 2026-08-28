import { captureException } from '@tinycld/core/lib/errors'
import { CONNECT_HREF } from '@tinycld/core/lib/org-routes'
import { disconnectServer } from '@tinycld/core/lib/pocketbase'
import { forgetServer } from '@tinycld/core/lib/remove-server'
import { getResolvedAddress } from '@tinycld/core/lib/server-address'
import { isSavedServersSupported } from '@tinycld/core/lib/servers'
import { router } from 'expo-router'
import { Pressable, Text, View } from 'react-native'

export function DisconnectServerSection() {
    async function onDisconnect() {
        const active = getResolvedAddress()
        // Forget THIS server rather than the only one: with several saved, the
        // user expects the others to survive. forgetServer drops this entry, its
        // token and its push registration, then either switches to the next
        // saved server or — if this was the last — falls back to the old
        // disconnect-and-route-to-/connect behavior.
        if (active && isSavedServersSupported()) {
            try {
                const outcome = await forgetServer(active)
                if (outcome.status === 'disconnected') router.replace(CONNECT_HREF)
                return
            } catch (err) {
                // A refused JS-context restart leaves this server removed but the
                // switch incomplete. Fall through to the plain disconnect so the
                // user still lands somewhere coherent.
                captureException('disconnect-server-section.forget', err)
            }
        }

        await disconnectServer()
        router.replace(CONNECT_HREF)
    }

    return (
        <View className="gap-3">
            <Text className="text-xl font-bold text-foreground">Server</Text>
            <View className="rounded-xl border border-border bg-surface-secondary p-4 gap-2">
                <Text className="text-base font-semibold text-foreground">Disconnect server</Text>
                <Text className="text-[13px] text-muted-foreground">
                    Sign out and forget this server. Your account on the server is not deleted, and
                    any other servers you've added stay signed in.
                </Text>
                <Pressable
                    onPress={onDisconnect}
                    className="self-start rounded-lg mt-1 px-3 py-2 border border-border"
                >
                    <Text className="text-foreground font-semibold">Disconnect</Text>
                </Pressable>
            </View>
        </View>
    )
}
