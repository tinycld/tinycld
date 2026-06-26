import { isUpdateTransportAllowed, postBootBeacon } from '@tinycld/core/lib/app-updater/client'
import { captureException } from '@tinycld/core/lib/errors'
import { getResolvedAddress } from '@tinycld/core/lib/server-address'
import AppUpdater from 'app-updater'
import { useEffect } from 'react'
import { Platform } from 'react-native'

// BundleSentinel reports — once the real provider tree has committed — that the
// running bundle's JS actually EXECUTED and mounted, by POSTing a boot beacon to
// the server (which logs `app-boot: rendered`, readable from _logs by the OTA
// e2e). This is the proof the native currentId flip alone cannot give: that the
// NEW bundle's JS ran, not just that the native loader promoted it.
//
// Why a server beacon and not console.log: the OTA path runs only in Release
// builds, where RN does NOT route console.* to os_log — so a console line is
// invisible to the harness. The server channel works in Release (it's how the
// currentId flip itself is observed). NOT __DEV__-gated for the same reason; it
// no-ops on web (no native updater) and until the server address resolves.
//
// Mounted next to MarkBundleHealthy inside <Providers> so it fires only when the
// real provider tree commits, not the blank gate placeholder. Best-effort: a
// failed beacon is captured, never disrupts the app. Renders nothing.
export function useBundleSentinel(): void {
    useEffect(() => {
        if (Platform.OS === 'web') return
        const serverUrl = getResolvedAddress()
        if (!serverUrl || !isUpdateTransportAllowed(serverUrl)) return
        void postBootBeacon({
            serverUrl,
            platform: Platform.OS === 'ios' ? 'ios' : 'android',
            id: AppUpdater.getCurrentBundleId(),
            hash: AppUpdater.getCurrentBundleHash(),
            fetchFn: fetch,
        }).catch(err => captureException('bundle-sentinel.boot-beacon', err))
    }, [])
}

export function BundleSentinel(): null {
    useBundleSentinel()
    return null
}
