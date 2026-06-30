import { disconnectServer } from '@tinycld/core/lib/pocketbase'
import { router } from 'expo-router'
import { Platform, Pressable, Text } from 'react-native'

export function ChangeServerLink() {
    // Web always talks to its own origin — there's no other server to switch to.
    if (Platform.OS === 'web') return null

    async function onPress() {
        await disconnectServer()
        router.replace('/connect?backTo=/')
    }

    return (
        <Pressable onPress={onPress} accessibilityRole="link">
            <Text className="text-xs text-muted-foreground underline">Change server</Text>
        </Pressable>
    )
}
