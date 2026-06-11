import { DocumentTitle } from '@tinycld/core/components/DocumentTitle'
import { navigateToOrg } from '@tinycld/core/lib/org-url'
import { setResolvedAddress, writeCached } from '@tinycld/core/lib/server-address'
import { useAuthStore } from '@tinycld/core/lib/stores/auth-store'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useRef, useState } from 'react'
import { ActivityIndicator, Linking, Pressable, Text, View } from 'react-native'

// A demo tap on tinycld.org (universal/app link) lands here on devices with the
// app installed — the AASA/assetlinks claim tinycld.org/demo, which +native-intent
// rewrites to this pre-auth public route (/p/demo). We always pin the public
// production server: browsing the marketing site signals wanting the hosted demo,
// not any self-hosted server the user may have configured. __DEV__ keeps whatever
// dev server is already resolved so local testing of `tinycld://p/demo` hits localhost.
const DEMO_SERVER = 'https://tinycld.org'

type DemoState = { status: 'starting' } | { status: 'error'; message: string }

function useStartDemo() {
    const [state, setState] = useState<DemoState>({ status: 'starting' })
    const startDemo = useAuthStore(s => s.startDemo)
    const started = useRef(false)

    async function run() {
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
    }

    // Kick off on first render. A ref guards against double-invocation; this is a
    // navigation side-effect, not data sync, so an inline kickoff is appropriate.
    if (!started.current && state.status === 'starting') {
        void run()
    }

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
