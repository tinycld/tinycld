import type { Href } from 'expo-router'

type QueryParams = Record<string, string | number | string[]>

/**
 * Hook for navigation. Single-org deployment: routes no longer carry an
 * [orgSlug] segment, so paths are app-root-relative.
 *
 * Retained as a hook (not a plain function) so the ~200 call sites keep their
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
        const pathname = path === '' ? '/' : `/${path}`
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
