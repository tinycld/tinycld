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
        return {
            pathname,
            params: { ...extra },
        } as Href
    }
}
