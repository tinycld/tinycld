import AsyncStorage from '@react-native-async-storage/async-storage'
import { authKeyFor, parseAuthBlob } from './auth-storage'
import { captureException } from './errors'
import { unregisterExpoPushToken } from './expo-push'
import { getResolvedAddress } from './server-address'
import { isSameServer, removeServer } from './servers'
import { switchToServer } from './switch-server'

export type RemoveServerOutcome =
    | { status: 'removed' }
    | { status: 'switched'; origin: string }
    | { status: 'disconnected' }

// Cancel a server's push registration against THAT server, using that server's
// own stored credentials. Removal is a logout by another name, so it must not
// leave the removed server pushing to this device — and it must not delete the
// row on whichever server merely happens to be active.
async function cancelPushFor(origin: string): Promise<void> {
    try {
        const raw = await AsyncStorage.getItem(authKeyFor(origin))
        if (!raw) return
        const parsed = parseAuthBlob(raw)
        const record = (parsed?.record ?? parsed?.model) as { id?: string } | undefined
        if (!parsed?.token || !record?.id) return
        await unregisterExpoPushToken(record.id, parsed.token, origin)
    } catch (err) {
        // Best-effort, exactly as the logout path: a failed teardown must never
        // block the removal the user asked for.
        captureException('remove-server.cancelPush', err)
    }
}

// forgetServer removes a saved server: its list entry, its scoped auth blob, and
// its push registration. Handles the interesting case — removing the server that
// is currently ACTIVE — by defining where the user lands.
//
//  - Other servers remain  → switch to the next one (a full switch, so the
//    module graph rebinds; see switchToServer).
//  - It was the last one   → fall back to today's disconnect behavior and route
//    to /connect.
//  - Not the active server → just drop it; nothing about the running session
//    changes.
export async function forgetServer(origin: string): Promise<RemoveServerOutcome> {
    await cancelPushFor(origin)
    await AsyncStorage.removeItem(authKeyFor(origin))

    const remaining = await removeServer(origin)

    const active = getResolvedAddress()
    const wasActive = !!active && isSameServer(active, origin)
    if (!wasActive) return { status: 'removed' }

    const next = remaining[remaining.length - 1]
    if (next) {
        await switchToServer(next.origin)
        return { status: 'switched', origin: next.origin }
    }

    // Last server gone: tear the session down and drop the address so the gate
    // routes to the picker. This is the one path that still routes through
    // disconnectServer, whose logout() fires resetSessionState() un-awaited —
    // await it here rather than propagating that race.
    const { disconnectServer, resetSessionState } = await import('./pocketbase')
    await disconnectServer()
    await resetSessionState()
    return { status: 'disconnected' }
}
