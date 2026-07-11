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

// Tear down this device's Expo push registration on logout: delete the
// server push_subscriptions row(s) for this user's Expo token so the backend
// stops pushing to a device the user has signed out of. A no-op on web (no
// Expo token) and best-effort — a teardown failure must never block logout.
//
// `authToken` is passed explicitly (rather than read from pb.authStore) so the
// caller can run teardown as part of logout without racing pb.authStore.clear():
// we send it as the Authorization header so the delete is authorized even once
// the store has been cleared.
export async function unregisterExpoPushToken(userId: string, authToken?: string): Promise<void> {
    if (Platform.OS === 'web') return

    const authOptions = authToken ? { headers: { Authorization: authToken } } : {}
    try {
        const Notifications = await import('expo-notifications')
        const tokenData = await Notifications.getExpoPushTokenAsync()
        const token = tokenData.data

        // biome-ignore lint/plugin/pbtsdb-no-raw-pb-access: imperative push-teardown util (not a React hook); pairs with registerExpoPushToken above.
        const records = await pb.collection('push_subscriptions').getFullList({
            ...authOptions,
            filter: `user = "${userId}" && platform = "expo" && expo_token = "${token}"`,
        })
        for (const record of records) {
            // biome-ignore lint/plugin/pbtsdb-no-raw-pb-access: imperative push-teardown util (not a React hook); pairs with registerExpoPushToken above.
            await pb.collection('push_subscriptions').delete(record.id, authOptions)
        }
    } catch {
        // Best-effort: a failed teardown must never block logout.
    }
}
