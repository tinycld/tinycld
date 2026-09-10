import { APP_PREFIX, CONNECT_HREF } from '@tinycld/core/lib/org-routes'

/**
 * The route a signed-out user was trying to reach, held until they sign in.
 *
 * WHY THIS IS CAPTURED AT MODULE LOAD RATHER THAN WHEN THE GATE RENDERS:
 * expo-router derives the browser URL from the navigators that are MOUNTED, and
 * app/a/(app)/_layout can't mount the package tabs for a signed-out user. So on
 * a deep link the address bar collapses to the bare app root BEFORE the sign-in
 * form appears — the same "bounces through /a on every cold load" effect
 * described in app/_layout.tsx's ReadySlot comment. Reading the href at gate
 * time therefore yields '/a', not the link the user clicked. The entry URL is
 * only reliably available before any of that runs, so it is snapshotted here at
 * import time and consumed after sign-in.
 *
 * Deliberately NOT persisted: a destination written to disk outlives the intent
 * behind it and would teleport the user mid-launch days later. Native cold-start
 * deep links don't need it — expo-router hands the launch URL to the route tree
 * in the same session this module is loaded in.
 */
let pendingRoute: string | null = null

/** Routes that are already the post-login landing, or are pre-auth plumbing. */
const NEVER_RESTORED = new Set<string>(['/', APP_PREFIX, CONNECT_HREF, '/p/demo'])

/**
 * True for an href worth returning to after sign-in.
 *
 * The leading-slash test is the same open-redirect guard the `backTo` consumers
 * use (app/a/connect.tsx), plus the `//` case they miss: a browser reads a
 * protocol-relative `//evil.example.com` as an absolute off-site URL.
 */
function isRestorable(href: string): boolean {
    if (!href.startsWith('/') || href.startsWith('//')) return false
    return !NEVER_RESTORED.has(href.split('?', 1)[0])
}

export function setPendingRoute(href: string): void {
    if (isRestorable(href)) pendingRoute = href
}

/** Read and clear, so a later sign-out → sign-in can't resurrect a stale target. */
export function takePendingRoute(): string | null {
    const route = pendingRoute
    pendingRoute = null
    return route
}

export function clearPendingRoute(): void {
    pendingRoute = null
}

/**
 * Snapshot the URL the app was opened at, on web, before expo-router rewrites
 * it. Runs at import time; a no-op off-web and in any non-browser context
 * (SSR, unit tests), where `location` is absent.
 *
 * Native's launch URL arrives through +native-intent.ts and reaches the route
 * tree directly, so there is nothing to snapshot here for it — the same
 * captureCurrentRoute() call from the gate covers the cases where the mounted
 * route IS the destination.
 */
function captureEntryUrl(): void {
    if (typeof location === 'undefined') return
    setPendingRoute(`${location.pathname}${location.search}`)
}

captureEntryUrl()
