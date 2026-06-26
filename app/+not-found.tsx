import { trace } from '@tinycld/core/lib/debug-trace'
import { Link, useGlobalSearchParams, usePathname } from 'expo-router'
import { Text, View } from 'react-native'

// Custom not-found route. Replaces expo-router's built-in Unmatched screen (which
// links to the Sitemap view — that view crashes on native via
// window.location.origin). More importantly it TRACES the exact path that failed
// to match, so the post-login "Unmatched Route" can be diagnosed from server logs.
//
// TEMPORARY diagnostic + a safer fallback than the default. Keep the route; strip
// the trace() once the routing bug is fixed.
export default function NotFound() {
    const pathname = usePathname()
    const params = useGlobalSearchParams()

    trace('NOT-FOUND', { pathname, params: JSON.stringify(params) })

    return (
        <View className="flex-1 items-center justify-center gap-4 bg-background p-6">
            <Text className="text-2xl font-bold text-foreground">Page not found</Text>
            <Text className="text-sm text-muted-foreground">{pathname}</Text>
            <Link href="/" className="text-primary">
                Go home
            </Link>
        </View>
    )
}
