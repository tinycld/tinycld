import { Text, View } from 'react-native'

const PLACEHOLDER_CODE = 'XXXX-XXXX'

/**
 * Mirrors the box the server prints at start-up, so the person knows which
 * part of the log to look for.
 */
export function ServerLogPreview({ code }: { code: string }) {
    const shown = code || PLACEHOLDER_CODE
    return (
        <View className="w-full max-w-[340px] items-center gap-2.5">
            <View
                className="w-full rounded-xl bg-rail-background p-3.5"
                accessibilityLabel={`Server log showing setup code ${shown}`}
            >
                <View className="gap-0.5 border border-rail-text px-2 py-1.5">
                    <Text className="font-mono text-[11px] text-rail-text">
                        Finish setup in your browser:
                    </Text>
                    <View className="flex-row items-center">
                        <Text className="font-mono text-[11px] text-rail-text">Setup code: </Text>
                        <View className="rounded bg-primary px-1">
                            <Text className="font-mono text-[11px] font-bold text-primary-foreground">
                                {shown}
                            </Text>
                        </View>
                    </View>
                </View>
            </View>
            <Text className="text-center text-[11px] text-muted-foreground">
                Look for this box in the server log.
            </Text>
        </View>
    )
}
