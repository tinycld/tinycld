import type { Href } from 'expo-router'

type QueryParams = Record<string, string | number | string[]>

/**
 * The URL segment every app route lives under.
 *
 * A constant namespace for the app's own routes, so they can never collide
 * with a protocol mount (/dav, /caldav), the public share tree (/p), or a
 * marketing path. WebDAV at bare /drive versus the in-app /drive route was
 * exactly that collision — see isDavPath in core/server/coreserver/server.go.
 *
 * NOT an org slug: single-org deployments give each org its own host, so
 * nothing interpolates into this. It is one fixed segment.
 */
export const APP_PREFIX = '/a'

/**
 * Build an app href from a root-relative path. Plain function, so code outside
 * a component (route resolvers, redirect helpers) can reach it; useOrgHref
 * delegates here so there is one definition of the prefix.
 */
export function appHref(path: string): string {
    return path === '' ? APP_PREFIX : `${APP_PREFIX}/${path}`
}

/** Pre-auth routes whose literals are referenced from more than one module. */
export const CONNECT_HREF = appHref('connect')
export const PICK_ORG_HREF = appHref('pick-org')

/**
 * First path segments that were app routes before the move under APP_PREFIX.
 * Mirrors legacyAppSegments in core/server/coreserver/static.go and the set in
 * app/+native-intent.ts; keep the three in step.
 */
const LEGACY_APP_SEGMENTS = new Set([
    'accept-invite',
    'reset-password',
    'connect',
    'pick-org',
    'setup',
    'settings',
    'help',
])

/**
 * Normalize an inbound app path to the current prefix.
 *
 * For links minted BEFORE app routes moved under APP_PREFIX and still arriving
 * from outside the app — universal/deep links opened on a phone, notably the
 * emailed invite and password-reset links, which keep landing for days. The
 * web-side counterpart is legacyAppRedirect in core/server/coreserver/static.go.
 *
 * Anything not a recognizably legacy app path (already prefixed, public `/p/…`,
 * an absolute URL, a protocol mount) is returned untouched: this normalizes,
 * it does not rewrite arbitrary input.
 *
 * Not used for stored data — notification `url` rows are rewritten once by
 * pb_migrations/2010000000_prefix_notification_urls.js instead.
 */
export function normalizeLegacyAppPath(path: string): string {
    if (!path.startsWith('/') || path === APP_PREFIX || path.startsWith(`${APP_PREFIX}/`)) {
        return path
    }
    const segment = path.slice(1).split(/[/?]/, 1)[0]
    return LEGACY_APP_SEGMENTS.has(segment) ? `${APP_PREFIX}${path}` : path
}

/**
 * The active package slug for a pathname, or null outside a package.
 *
 * Segment 1 is always APP_PREFIX, so the slug is segment 2. Derived from
 * APP_PREFIX rather than a literal so it stays in step if the prefix moves.
 */
export function activeSlugFromPathname(pathname: string): string | null {
    const prefix = `${APP_PREFIX}/`
    if (!pathname.startsWith(prefix)) return null
    return pathname.slice(prefix.length).split(/[/?]/, 1)[0] || null
}

/**
 * Hook for navigation. Single-org deployment: routes carry no [orgSlug]
 * segment, so paths are relative to the app root (APP_PREFIX).
 *
 * Retained as a hook (not a plain function) so the ~250 call sites keep their
 * shape; it no longer needs org context.
 *
 * Usage:
 *   const orgHref = useOrgHref()
 *   router.push(orgHref('contacts/new'))
 *   router.push(orgHref('contacts/[id]', { id: '123' }))
 *   router.push(orgHref('mail', { folder: 'sent' }))
 *   <Link href={orgHref('mail/[id]', { id: threadId })} />
 */
export function useOrgHref() {
    return (path: string, extra?: QueryParams): Href => {
        const pathname = appHref(path)
        // Return a plain string href when there are no query params. A string is
        // a stable, referentially-equal value across renders; the object form
        // (`{ pathname, params }`) is a NEW object every call, which makes
        // <Redirect href={...}> / expo-router re-navigate on every render and can
        // drive React Navigation into an infinite setState loop (React #185).
        // Only build the object when params are actually present (dynamic routes).
        if (!extra || Object.keys(extra).length === 0) {
            return pathname as Href
        }
        return { pathname, params: extra } as Href
    }
}
