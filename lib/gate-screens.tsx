import { MinimalProviders } from '@tinycld/core/components/MinimalProviders'
import { Slot } from 'expo-router'
import { Text, View } from 'react-native'

// Presentational screens the layout shows while the server-address gate is still
// working (see useServerAddressGate). Each wraps MinimalProviders — the minimal
// theme/safe-area shell available before the real Providers module has loaded.

// BlankScreen is the neutral placeholder shown while resolving, or when
// unresolved on a non-/connect route (the redirect effect is already navigating).
export function BlankScreen() {
    return (
        <MinimalProviders>
            <View className="flex-1 bg-background" />
        </MinimalProviders>
    )
}

// ConnectSlot renders the /connect route itself while the address is unresolved —
// the one unresolved route the user is allowed to see (it's how they set the
// address). Rendering <Slot /> here lets the /connect screen mount inside the
// minimal shell instead of being redirected away from.
export function ConnectSlot() {
    return (
        <MinimalProviders>
            <Slot />
        </MinimalProviders>
    )
}

// GateFailedScreen surfaces a fatal boot error (e.g. the dynamic Providers import
// threw) instead of leaving a blank white screen with nothing in the logs.
export function GateFailedScreen({ error }: { error: string }) {
    return (
        <MinimalProviders>
            <View className="flex-1 items-center justify-center bg-background gap-2 p-6">
                <Text className="text-foreground" style={{ fontSize: 18, fontWeight: '600' }}>
                    Failed to load app
                </Text>
                <Text className="text-muted-foreground text-center">{error}</Text>
            </View>
        </MinimalProviders>
    )
}
