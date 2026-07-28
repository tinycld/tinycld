import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { DEFAULT_COLOR_THEME } from '@tinycld/core/lib/color-themes'
import { GluestackUIProvider } from '@tinycld/core/ui/gluestack-ui-provider'
import type { ReactNode } from 'react'
import { GestureHandlerRootView } from 'react-native-gesture-handler'
import { SafeAreaProvider } from 'react-native-safe-area-context'

// Provider stack for screens that must render before a server address is
// resolved (e.g. the /connect onboarding screen). Skips PBTSDB / Auth /
// Shortcuts because those depend on PB_SERVER_ADDR; falls back to static
// theme defaults instead of useThemePreference + useColorTheme, both of
// which require PocketBase.
//
// Own QueryClient rather than the shared one from lib/pocketbase: importing
// that module here would drag the PocketBase client into the pre-address
// screens this stack exists to keep it out of. Queries that run before the
// address resolves (e.g. useOrgInfo) fail closed to their empty states.
const minimalQueryClient = new QueryClient()

export function MinimalProviders({ children }: { children: ReactNode }) {
    return (
        <GestureHandlerRootView className="flex-1">
            <SafeAreaProvider>
                <GluestackUIProvider mode="system" colorTheme={DEFAULT_COLOR_THEME}>
                    <QueryClientProvider client={minimalQueryClient}>
                        {children}
                    </QueryClientProvider>
                </GluestackUIProvider>
            </SafeAreaProvider>
        </GestureHandlerRootView>
    )
}
