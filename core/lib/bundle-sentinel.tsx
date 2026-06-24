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
