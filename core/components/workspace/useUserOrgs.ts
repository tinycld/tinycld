import { orgUrlForSlug, parseOrgsCookie } from '@tinycld/core/lib/org-cookie'
import { Platform } from 'react-native'

// The orgs this browser has signed into, from the parent-domain cookie each
// router-managed tenant upserts at login (see lib/org-cookie.ts for the
// contract). Web-only: the cookie lives on the browser's parent domain, and a
// native app talks to exactly one server. On a standalone deployment (no
// router, no cookie) this is [] and the switcher UI renders nothing.
export interface UserOrgEntry {
    id: string
    name: string
    slug: string
    /** Absolute URL of the org's own origin — derived from the slug and this
     *  page's own parent domain, never read from the (client-writable)
     *  cookie. */
    url: string
}

export function useUserOrgs(): UserOrgEntry[] {
    if (Platform.OS !== 'web' || typeof document === 'undefined') return []
    const hostname = window.location.hostname
    const entries: UserOrgEntry[] = []
    for (const entry of parseOrgsCookie(document.cookie)) {
        const url = orgUrlForSlug(entry.slug, hostname)
        if (!url) continue
        entries.push({ id: entry.slug, name: entry.name, slug: entry.slug, url })
    }
    return entries
}

/** Whether an org entry points at the origin the app is currently served
 *  from — the "you are here" highlight in the switcher. */
export function isCurrentOrg(entry: UserOrgEntry): boolean {
    if (typeof window === 'undefined') return false
    try {
        return new URL(entry.url).origin === window.location.origin
    } catch {
        return false
    }
}
