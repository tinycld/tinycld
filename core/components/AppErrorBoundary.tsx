import type { ErrorBoundaryProps } from 'expo-router'
import { AlertTriangle } from 'lucide-react-native'
import { useEffect } from 'react'
import { Pressable, ScrollView, Text, View } from 'react-native'
import { captureException } from '../lib/errors'
import { symbolicateStack } from '../lib/symbolicate-stack'
import { useThemeColor } from '../lib/use-app-theme'

// Root error boundary. Exported as `ErrorBoundary` from app/_layout.tsx, which
// makes Expo Router wrap the whole route subtree (<Slot />) in it — so any
// uncaught error a screen throws during render lands here instead of
// white-screening the app.
//
// On a caught error it:
//   1. Reports to Sentry via captureException (Sentry symbolicates the minified
//      stack server-side using the emitted sourcemaps).
//   2. Symbolicates the stack CLIENT-SIDE against the served `.map` files and
//      console.logs the readable, original-source stack — so a developer
//      watching the console sees `…/crash-test.tsx:30` instead of
//      `…/entry-abc.js:258:53709`, with no DevTools or Sentry round-trip.
//   3. Shows a recoverable fallback instead of a blank page.
//
// The raw stack is also rendered into a testID'd element: it's a stack trace,
// not user data, so it's safe to show, it helps anyone who hits the screen file
// a useful report, and the sourcemaps e2e spec reads it.
export function AppErrorBoundary({ error, retry }: ErrorBoundaryProps) {
    // Report + symbolicate once per distinct error. Expo Router's <Try> only
    // re-renders this boundary when the caught error changes, so keying the
    // effect on `error` runs it exactly once per crash (and not on unrelated
    // re-renders).
    useEffect(() => {
        captureException('app.errorBoundary', error)
        if (!error.stack) return
        symbolicateStack(error.stack)
            .then(clean => {
                // Load-bearing in ALL builds: the browser never applies
                // sourcemaps to error.stack at runtime (only DevTools/Sentry do),
                // so this is the ONLY place the readable, source-mapped crash
                // stack surfaces in a production web build. sourcemaps.spec.ts
                // asserts this line fires and names the original source. The file
                // is exempt from noConsole in biome.json (diagnostic-modules
                // override).
                console.log(`[error-boundary] ${clean}`)
            })
            .catch(() => {
                // Symbolication is best-effort; the raw error already went to
                // Sentry and is shown on screen.
            })
    }, [error])

    const dangerColor = useThemeColor('danger')

    return (
        <View
            testID="error-boundary"
            className="flex-1 items-center justify-center bg-background p-6"
        >
            <View className="w-full max-w-lg items-center gap-4">
                <AlertTriangle size={36} color={dangerColor} />
                <Text className="text-lg font-semibold text-foreground text-center">
                    Something went wrong
                </Text>
                <Text className="text-sm text-muted-foreground text-center">
                    The screen hit an unexpected error. It's been reported.
                </Text>
                <Pressable
                    testID="error-boundary-retry"
                    onPress={retry}
                    accessibilityRole="button"
                    className="mt-2 rounded-lg bg-primary px-4 py-2"
                >
                    <Text className="text-sm font-medium text-primary-foreground">Try again</Text>
                </Pressable>
                {/* Raw (un-symbolicated) stack. Capped height so a long stack
                    stays scrollable rather than pushing the retry button off. */}
                <ScrollView
                    testID="error-boundary-stack"
                    className="max-h-40 w-full rounded-lg bg-muted p-3"
                >
                    <Text className="text-xs text-muted-foreground" selectable>
                        {error.stack ?? error.message}
                    </Text>
                </ScrollView>
            </View>
        </View>
    )
}
