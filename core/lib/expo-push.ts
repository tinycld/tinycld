import { Platform } from 'react-native'
import { pb } from './pocketbase'

export async function registerExpoPushToken(userId: string): Promise<boolean> {
    if (Platform.OS === 'web') return false

    try {
        const Notifications = await import('expo-notifications')

        const { status: existingStatus } = await Notifications.getPermissionsAsync()
        let finalStatus = existingStatus
        if (existingStatus !== 'granted') {
            const { status } = await Notifications.requestPermissionsAsync()
            finalStatus = status
        }
        if (finalStatus !== 'granted') return false

        const tokenData = await Notifications.getExpoPushTokenAsync()
        const token = tokenData.data

        // Check if this token is already registered
        // biome-ignore lint/plugin/pbtsdb-no-raw-pb-access: imperative push-registration util (not a React hook); push_subscriptions isn't a store-backed collection here.
        const existing = await pb.collection('push_subscriptions').getFullList({
            filter: `user = "${userId}" && platform = "expo" && expo_token = "${token}"`,
        })
        if (existing.length > 0) return true

        // biome-ignore lint/plugin/pbtsdb-no-raw-pb-access: imperative push-registration util (not a React hook), so useMutation/pbtsdb can't apply; runs after a native permission grant.
        await pb.collection('push_subscriptions').create({
            user: userId,
            platform: 'expo',
            expo_token: token,
            endpoint: `expo://${token}`,
            keys: {},
        })
        return true
    } catch {
        return false
    }
}
