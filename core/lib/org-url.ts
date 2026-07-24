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
