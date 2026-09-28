import { Text, View } from 'react-native'

const PLACEHOLDER_CODE = 'XXXX-XXXX'

/**
 * Mirrors the box the server prints at start-up, so the person knows which
 * part of the log to look for.
 */
export function ServerLogPreview({ code }: { code: string }) {
    const shown = code || PLACEHOLDER_CODE
    return (
        <View
            className="gap-2 rounded-lg bg-rail-background px-4 py-3.5"
            accessibilityLabel={`Server log showing setup code ${shown}`}
        >
            <View className="h-1.5 w-1/3 rounded bg-rail-text/25" />
            <View className="gap-1 self-start border border-rail-text px-3 py-2">
                <Text className="font-mono text-xs text-rail-active-text">
                    Finish setup in your browser:
                </Text>
                <View className="flex-row items-center">
                    <Text className="font-mono text-xs text-rail-active-text">Setup code: </Text>
                    <View className="rounded bg-primary px-1">
                        <Text className="font-mono text-xs font-bold text-primary-foreground">
                            {shown}
                        </Text>
                    </View>
                </View>
            </View>
            <View className="h-1.5 w-1/4 rounded bg-rail-text/25" />
        </View>
    )
}
