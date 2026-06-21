import { useLocalSearchParams } from 'expo-router'
import { useState } from 'react'
import { Pressable, Text, View } from 'react-native'

// Deterministic crash site for the sourcemap e2e test (tests/e2e/sourcemaps.spec.ts).
//
// It throws ONLY when asked — either ?boom=1 in the URL or after pressing the
// button — so the route is otherwise inert and navigating to /crash-test does
// nothing surprising. The throw is a render-phase error, so Expo Router's root
// ErrorBoundary (app/_layout.tsx → AppErrorBoundary) catches it; the test then
// reads the captured stack, fetches the served .map for the generated frame,
// and resolves it back to THIS file to prove sourcemaps are built + served +
// correct.
//
// detonate() is a separate named function on its own line so the thrown stack
// has a stable top frame pointing at a known location in this file.
const CRASH_MESSAGE = 'crash-test: intentional render crash for sourcemap e2e'

function detonate(): never {
    throw new Error(CRASH_MESSAGE)
}

export default function CrashTest() {
    const { boom } = useLocalSearchParams<{ boom?: string }>()
    const [armed, setArmed] = useState(false)

    if (boom === '1' || armed) {
        detonate()
    }

    return (
        <View
            testID="crash-test"
            className="flex-1 items-center justify-center gap-4 bg-background"
        >
            <Text className="text-foreground">Crash test route</Text>
            <Pressable
                testID="crash-test-trigger"
                onPress={() => setArmed(true)}
                accessibilityRole="button"
                className="rounded-lg bg-danger px-4 py-2"
            >
                <Text className="text-danger-foreground">Trigger crash</Text>
            </Pressable>
        </View>
    )
}
