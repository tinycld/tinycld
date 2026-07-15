import { DocumentTitle } from '@tinycld/core/components/DocumentTitle'
import { navigateToOrg } from '@tinycld/core/lib/org-url'
import { DEMO_SERVER, setResolvedAddress, writeCached } from '@tinycld/core/lib/server-address'
import { useAuthStore } from '@tinycld/core/lib/stores/auth-store'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useCallback, useEffect, useRef, useState } from 'react'
import { ActivityIndicator, Linking, Pressable, Text, View } from 'react-native'

// A demo tap on tinycld.org (universal/app link) lands here on devices with the
// app installed — the AASA/assetlinks claim tinycld.org/demo, which +native-intent
// rewrites to this pre-auth public route (/p/demo). We always pin the public
// production server (DEMO_SERVER): browsing the marketing site signals wanting the
// hosted demo, not any self-hosted server the user may have configured. __DEV__
// keeps whatever dev server is already resolved so local testing of `tinycld://p/demo`
// hits localhost. The server-address gate seeds the same DEMO_SERVER synchronously
// for /p/demo so this screen mounts inside the provider tree on a fresh install.

type DemoState = { status: 'starting' } | { status: 'error'; message: string }

function useStartDemo() {
    const [state, setState] = useState<DemoState>({ status: 'starting' })
    const startDemo = useAuthStore(s => s.startDemo)
    const started = useRef(false)

    const run = useCallback(async () => {
        if (started.current) return
        started.current = true
        setState({ status: 'starting' })

        const server = __DEV__ ? null : DEMO_SERVER
        if (server) {
            setResolvedAddress(server)
            await writeCached(server)
        }

        const target = server ?? DEMO_SERVER
        const { error } = await startDemo(target)
        if (error) {
            started.current = false
            setState({ status: 'error', message: error })
            return
        }
        navigateToOrg('demo')
    }, [startDemo])

    // Kick off in an effect (not during render): starting the demo fires a
    // network request + navigation, so a discarded / strict-mode render must
    // not trigger it for a tree that never commits. The started.current ref
    // still guards against a second invocation (incl. strict-mode's double
    // effect), so the demo starts exactly once when status becomes 'starting'.
    useEffect(() => {
        if (state.status === 'starting') void run()
    }, [state.status, run])

    return { state, retry: run }
}

export default function StartDemo() {
    const { state, retry } = useStartDemo()

    return (
        <View className="flex-1 items-center justify-center p-5 bg-background">
            <DocumentTitle title="Starting demo" includeOrg={false} />
            {state.status === 'starting' ? <StartingCard /> : null}
            {state.status === 'error' ? (
                <ErrorCard message={state.message} onRetry={retry} />
            ) : null}
        </View>
    )
}

function StartingCard() {
    const muted = useThemeColor('muted-foreground')
    return (
        <View className="items-center gap-4">
            <ActivityIndicator size="large" color={muted} />
            <Text className="text-sm text-muted-foreground">Starting your demo…</Text>
        </View>
    )
}

function ErrorCard({ message, onRetry }: { message: string; onRetry: () => void }) {
    return (
        <View
            className="gap-4 p-6 rounded-xl border border-border items-center bg-surface-secondary"
            style={{ maxWidth: 400, width: '100%' }}
        >
            <Text className="text-lg font-semibold text-foreground">Couldn't start the demo</Text>
            <Text className="text-center text-sm text-muted-foreground">{message}</Text>
            <Pressable
                onPress={onRetry}
                className="rounded-xl bg-foreground px-5 py-3 items-center w-full"
            >
                <Text className="text-sm font-semibold text-background">Try again</Text>
            </Pressable>
            <Pressable onPress={() => Linking.openURL(`${DEMO_SERVER}/demo?web=1`)}>
                <Text className="text-sm text-primary">Open the demo in your browser</Text>
            </Pressable>
        </View>
    )
}
