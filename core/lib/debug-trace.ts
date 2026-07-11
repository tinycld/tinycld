import { getResolvedAddress } from '@tinycld/core/lib/server-address'
import { Platform } from 'react-native'

// TEMPORARY routing/boot debug tracer. Release builds do NOT route console.* to
// os_log, so the only channel observable on a real device is a network POST to
// our own server (the same trick BundleSentinel's boot beacon uses). This POSTs
// each checkpoint to /api/app/boot, which logs `app-boot: rendered` with the
// message in the q.bundleId field — readable from the server's _logs:
//
//   docker logs <container> 2>&1 | grep 'TRACE:'
//   (or PB admin → _logs, filter "app-boot: rendered")
//
// Fire-and-forget; never throws, never blocks render. REMOVE once the routing
// bug is found — this is a debug aid, not a shipping feature.
//
// Both the console line AND the network beacon are __DEV__-gated: a release
// build must NEVER beacon (it would POST to /api/app/boot on every trace call,
// including every navigateToOrg, for all users).
export function trace(message: string, extra?: Record<string, unknown>): void {
    if (!__DEV__) return

    const serverUrl = getResolvedAddress()
    const line = extra ? `TRACE: ${message} ${JSON.stringify(extra)}` : `TRACE: ${message}`
    // biome-ignore lint/suspicious/noConsole: temporary debug tracer
    console.log(`[trace] ${line}`)
    if (!serverUrl) return
    void fetch(`${serverUrl}/api/app/boot`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
            id: line.slice(0, 500),
            platform: Platform.OS === 'ios' ? 'ios' : 'android',
            hash: '',
        }),
    }).catch(() => {
        // best-effort: a failed trace must never disrupt the app
    })
}

declare const __DEV__: boolean
