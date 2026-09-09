import { getCoreConfigOptional } from '@tinycld/core/lib/core-config'
import { Platform } from 'react-native'

// `pb` is imported lazily (not at module top level) to break the require cycle
// pocketbase → errors → notify → … → web-push → pocketbase. web-push only needs
// pb at call time inside these async actions, so a dynamic import is free here
// and matches the same cycle-breaking pattern used in pocketbase.ts.
//
// This is an imperative ServiceWorker/PushManager utility (not a React hook), so
// useMutation/pbtsdb primitives can't apply — the whole file is exempted from the
// pbtsdb-no-raw-pb-access plugin in biome.json.

export function isPushSupported(): boolean {
    return (
        Platform.OS === 'web' &&
        typeof navigator !== 'undefined' &&
        'serviceWorker' in navigator &&
        'PushManager' in window
    )
}

/**
 * The VAPID public key the browser signs its subscription against, injected by
 * the app server from the operator's system_settings (see
 * coreserver/static.go::publicConfigScript).
 *
 * This is deliberately NOT a build-time env read. It used to be
 * `process.env.VITE_VAPID_PUBLIC_KEY` — a name nothing ever defined (VITE_* is
 * Vite's prefix; this app bundles with Metro, which only inlines EXPO_PUBLIC_*),
 * so subscribeToPush bailed before it ever called pushManager.subscribe and the
 * settings toggle silently did nothing. A build constant is also the wrong shape
 * for the value: each deployment generates its own keypair after deploy.
 */
export function vapidPublicKey(): string {
    return getCoreConfigOptional()?.vapidPublicKey ?? ''
}

/**
 * Whether this browser supports push AND the server has web push configured.
 * Distinguishing the two lets the UI say "your browser can't" separately from
 * "this server hasn't set it up", instead of showing one dead toggle.
 */
export function isPushConfigured(): boolean {
    return isPushSupported() && vapidPublicKey() !== ''
}

export async function registerServiceWorker(): Promise<ServiceWorkerRegistration | null> {
    if (!isPushSupported()) return null
    return navigator.serviceWorker.register('/sw.js')
}

export async function subscribeToPush(userId: string): Promise<boolean> {
    if (!isPushSupported()) return false

    // Throw rather than return false: a missing key is an operator
    // misconfiguration the user can't fix by toggling again, and a silent false
    // is what made this failure invisible for so long. The caller surfaces it.
    const publicKey = vapidPublicKey()
    if (!publicKey) {
        throw new Error(
            'Web push is not configured on this server — an administrator must generate a VAPID keypair in Setup → Settings.'
        )
    }

    const registration = await registerServiceWorker()
    if (!registration) return false

    const subscription = await registration.pushManager.subscribe({
        userVisibleOnly: true,
        applicationServerKey: urlBase64ToUint8Array(publicKey) as BufferSource,
    })

    const subscriptionJSON = subscription.toJSON()

    const { pb } = await import('./pocketbase')
    await pb.collection('push_subscriptions').create({
        user: userId,
        endpoint: subscriptionJSON.endpoint,
        keys: {
            p256dh: subscriptionJSON.keys?.p256dh,
            auth: subscriptionJSON.keys?.auth,
        },
        user_agent: navigator.userAgent.slice(0, 500),
        platform: 'web',
    })

    return true
}

// `authToken` is passed explicitly (rather than read from pb.authStore) so the
// caller can run teardown as part of logout without racing pb.authStore.clear():
// when provided it's sent as the Authorization header so the delete is
// authorized even once the store has been cleared. The in-app settings toggle
// omits it and relies on the live session.
export async function unsubscribeFromPush(userId: string, authToken?: string): Promise<void> {
    if (!isPushSupported()) return

    const registration = await navigator.serviceWorker.ready
    const subscription = await registration.pushManager.getSubscription()
    if (subscription) {
        await subscription.unsubscribe()

        const authOptions = authToken ? { headers: { Authorization: authToken } } : {}
        const { pb } = await import('./pocketbase')
        const records = await pb.collection('push_subscriptions').getFullList({
            ...authOptions,
            filter: pb.filter('user = {:userId} && endpoint = {:endpoint}', {
                userId,
                endpoint: subscription.endpoint,
            }),
        })
        for (const record of records) {
            await pb.collection('push_subscriptions').delete(record.id, authOptions)
        }
    }
}

function urlBase64ToUint8Array(base64String: string): Uint8Array {
    const padding = '='.repeat((4 - (base64String.length % 4)) % 4)
    const base64 = (base64String + padding).replace(/-/g, '+').replace(/_/g, '/')
    const rawData = atob(base64)
    const outputArray = new Uint8Array(rawData.length)
    for (let i = 0; i < rawData.length; ++i) {
        outputArray[i] = rawData.charCodeAt(i)
    }
    return outputArray
}
