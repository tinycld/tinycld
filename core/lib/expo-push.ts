import PocketBase from 'pocketbase'
import { Platform } from 'react-native'
import { pb } from './pocketbase'

// These are imperative push-registration/teardown utilities (not React hooks),
// and push_subscriptions isn't a store-backed pbtsdb collection here, so
// useMutation/pbtsdb primitives can't apply — the whole file is exempted from the
// pbtsdb-no-raw-pb-access plugin in biome.json.

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
        const existing = await pb.collection('push_subscriptions').getFullList({
            filter: pb.filter('user = {:userId} && platform = "expo" && expo_token = {:token}', {
                userId,
                token,
            }),
        })
        if (existing.length > 0) return true

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
//
// `origin` is likewise explicit, and for a sharper reason. The shared `pb`
// points at whichever server is ACTIVE, but a device can hold sessions on
// several servers at once — so signing out of, or removing, a NON-ACTIVE server
// would issue this delete against the wrong host: the signed-out server keeps
// pushing while a server the user is still using is silently deregistered.
// Given an origin we build a short-lived client against that specific host
// (use-superuser-pb.ts is the precedent for a second client). Omitted → the
// active server, which is correct for the ordinary logout path.
export async function unregisterExpoPushToken(
    userId: string,
    authToken?: string,
    origin?: string
): Promise<void> {
    if (Platform.OS === 'web') return

    const authOptions = authToken ? { headers: { Authorization: authToken } } : {}
    try {
        const Notifications = await import('expo-notifications')
        const tokenData = await Notifications.getExpoPushTokenAsync()
        const token = tokenData.data

        const client = origin ? new PocketBase(origin) : pb
        const records = await client.collection('push_subscriptions').getFullList({
            ...authOptions,
            filter: client.filter(
                'user = {:userId} && platform = "expo" && expo_token = {:token}',
                { userId, token }
            ),
        })
        for (const record of records) {
            await client.collection('push_subscriptions').delete(record.id, authOptions)
        }
    } catch {
        // Best-effort: a failed teardown must never block logout.
    }
}
