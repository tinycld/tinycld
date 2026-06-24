import AppUpdater from 'app-updater'
import type { ReactNode } from 'react'
import { useEffect } from 'react'
import { Platform, Text, View } from 'react-native'

// BundleSentinel emits, on every real-tree mount (production included), a proof
// that the running bundle's JS actually executed and rendered — closing the gap
// where the native currentId flip alone does not prove the new JS ran/rendered.
// It logs a distinctive, scrapeable console line AND renders an accessibility-
// visible (visually negligible) element carrying the running bundle id, so an
// external harness can assert BOTH "JS executed + mounted" (console) and "on
// screen" (a11y tree). Mounted next to MarkBundleHealthy inside <Providers> so it
// fires only when the real provider tree commits, not the blank gate placeholder.
//
// NOT __DEV__-gated: the OTA path runs only in Release builds, where __DEV__ code
// is stripped. It DOES no-op on web (the app-updater module is stubbed there).

// First 12 chars of the bundle hash — enough to disambiguate; the full hash is
// unnecessary on screen / in the log.
export function shortHash(hash: string): string {
    return hash.slice(0, 12)
}

// The accessibilityLabel the harness reads back from the a11y tree.
export function formatSentinelLabel(bundleId: string): string {
    return `bundle:${bundleId}`
}

// The single console line the boot-log scraper greps for. Stable by contract:
// scripts/ota-e2e/boot-log-scraper.ts parses exactly this shape.
export function bootLogLine(bundleId: string, hash: string): string {
    return `[tinycld] app-boot: rendered bundle id=${bundleId} hash=${shortHash(hash)}`
}

// Logs the boot line exactly once per mount. Production-included (no __DEV__
// guard); no-ops on web where the native updater is stubbed and there is no OTA.
export function useBundleSentinel(): void {
    useEffect(() => {
        if (Platform.OS === 'web') return
        // console.log (not console.debug): `simctl log show` captures the default
        // os_log level; debug is filtered out unless verbose logging is enabled.
        console.log(bootLogLine(AppUpdater.getCurrentBundleId(), AppUpdater.getCurrentBundleHash()))
    }, [])
}

// BundleSentinel logs the boot proof and renders a visually-negligible,
// accessibility-visible element carrying the running bundle id, so a harness can
// assert the update is live on screen. Renders nothing on web. testID maps to the
// iOS accessibilityIdentifier that `idb ui describe-all` reports as AXIdentifier.
export function BundleSentinel(): ReactNode {
    useBundleSentinel()
    if (Platform.OS === 'web') return null
    const label = formatSentinelLabel(AppUpdater.getCurrentBundleId())
    // opacity:0 keeps it out of the visible UI while remaining in the iOS a11y
    // tree; pointerEvents:none so it never intercepts touches. If a future iOS
    // prunes fully-transparent nodes, bump opacity to 0.01. `accessible` makes the
    // View itself the single a11y element idb reports (testID → AXIdentifier,
    // accessibilityLabel → AXLabel) — the Text is a visual-only child with no label
    // of its own, so the harness reads exactly one node.
    return (
        <View
            accessible
            testID="ota-bundle-sentinel"
            accessibilityLabel={label}
            pointerEvents="none"
            style={{ position: 'absolute', top: 0, left: 0, width: 1, height: 1, opacity: 0 }}
        >
            <Text>{label}</Text>
        </View>
    )
}
