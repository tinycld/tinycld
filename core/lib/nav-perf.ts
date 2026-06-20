/**
 * Navigation timing. Marks the moment a tab is pressed and logs the elapsed
 * time at later milestones (first paint of the target screen, query settled)
 * so we can measure perceived vs. real tab-switch latency on a device.
 *
 * Gated on NAV_PERF rather than `__DEV__` so the markers can be measured in a
 * Release build (where `__DEV__` is false) without rebuilding to toggle them.
 * Call sites guard their argument construction with NAV_PERF too; the helpers
 * also short-circuit internally so a stray call is still cheap.
 *
 * Default OFF. To capture production first-mount timings: set NAV_PERF = true
 * AND switch the console.debug calls below to console.log (so they surface in
 * logcat from a Release build), then `expo run:android --variant release
 * --no-bundler`, kill Metro, launch from the embedded bundle, and
 * `adb logcat | grep '[nav-perf]'`. Leave NAV_PERF = false on any shipped
 * build — these must not run on the navigation hot path in production.
 */
export const NAV_PERF = false

let pressedSlug: string | null = null
let pressedAt = 0

export function markNavPress(slug: string) {
    if (!NAV_PERF) return
    pressedSlug = slug
    pressedAt = performance.now()
    console.debug(`[nav-perf] press → ${slug} @ t0`)
}

export function markNavMilestone(slug: string, label: string) {
    if (!NAV_PERF) return
    if (slug !== pressedSlug || pressedAt === 0) return
    const ms = Math.round(performance.now() - pressedAt)
    console.debug(`[nav-perf] ${slug} · ${label} +${ms}ms`)
}
