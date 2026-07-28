import { trace } from '@tinycld/core/lib/debug-trace'
import { router } from 'expo-router'

// Single-org deployment: routes no longer carry an [orgSlug] segment. These
// helpers keep their signatures (the org slug argument is ignored) so existing
// call sites compile; navigation is app-root-relative.

function getOrgPath(): string {
    return '/'
}

export function getOrgHrefString(_orgSlug?: string): string {
    return getOrgPath()
}

export function navigateToOrg(_orgSlug?: string): void {
    const path = getOrgPath()
    trace('navigateToOrg push', { path })
    router.push(path)
}

// Cross-org navigation is a full page load on the target org's own origin —
// each org is its own process, DB and client bundle, so there is no in-app
// route to it. The URL comes from the parent-domain switcher cookie
// (lib/org-cookie.ts). No-op outside the browser.
export function navigateToOrgUrl(url: string): void {
    trace('navigateToOrgUrl assign', { url })
    if (typeof window !== 'undefined') window.location.assign(url)
}
