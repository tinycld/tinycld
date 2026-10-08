import type { ConnectionIndicatorState } from '@tinycld/core/lib/connection-indicator'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useConnectionIndicator } from '@tinycld/core/lib/use-connection-indicator'
import { RefreshCw, WifiOff } from 'lucide-react-native'
import { Text, View } from 'react-native'

const COPY: Record<Exclude<ConnectionIndicatorState, 'hidden'>, string> = {
    offline: 'Offline — waiting for a connection',
    reconnecting: 'Reconnecting…',
}

/**
 * A small, non-blocking pill at the top of the app shell that says why lists
 * are still loading: no network, or the server not answering. It never takes
 * input (`pointerEvents="none"`), so the app stays usable underneath. See
 * useConnectionIndicator for when it shows.
 */
export function ConnectionIndicator() {
    const state = useConnectionIndicator()
    const iconColor = useThemeColor('warning')
    if (state === 'hidden') return null
    const Icon = state === 'offline' ? WifiOff : RefreshCw
    const label = COPY[state]

    return (
        <View
            pointerEvents="none"
            className="absolute top-2 left-0 right-0 items-center z-50"
            testID="connection-indicator"
        >
            <View
                accessibilityRole="alert"
                accessibilityLiveRegion="polite"
                accessibilityLabel={label}
                className="flex-row items-center gap-2 rounded-full border border-border bg-background px-3 py-1.5 shadow-sm"
            >
                <Icon size={14} color={iconColor} />
                <Text className="text-xs font-medium text-foreground">{label}</Text>
            </View>
        </View>
    )
}
