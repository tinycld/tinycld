// The cross-org switcher cookie. Each router-managed tenant upserts its own
// {slug, name, url} entry at login, scoped to the parent domain
// (Domain=.<MT_BASE_DOMAIN>), so the browser accumulates the orgs this user
// has actually signed into. It is a NAVIGATION HINT, not an authorization
// claim — it is client-writable, and every target org still authenticates the
// user itself. The server-side writer is serve-org (multi-org repo,
// internal/orgcookie); the two must agree on this shape.

export const ORGS_COOKIE_NAME = 'tinycld_orgs'

export interface CrossOrgEntry {
    slug: string
    name: string
    url: string
}

// Entries beyond this are dropped (oldest first) — matches the server-side
// writer's cap so the cookie can't grow unboundedly.
const MAX_ENTRIES = 20

/** Parses the tinycld_orgs cookie out of a document.cookie string. Malformed
 *  or incomplete entries are dropped rather than surfaced — a bad cookie must
 *  degrade to "no switcher", never to a broken menu. */
export function parseOrgsCookie(cookieHeader: string): CrossOrgEntry[] {
    const raw = cookieHeader
        .split(';')
        .map(part => part.trim())
        .find(part => part.startsWith(`${ORGS_COOKIE_NAME}=`))
        ?.slice(ORGS_COOKIE_NAME.length + 1)
    if (!raw) return []

    let parsed: unknown
    try {
        parsed = JSON.parse(decodeURIComponent(raw))
    } catch {
        return []
    }
    if (!Array.isArray(parsed)) return []

    const entries: CrossOrgEntry[] = []
    for (const item of parsed) {
        if (typeof item !== 'object' || item === null) continue
        const { slug, name, url } = item as Record<string, unknown>
        if (typeof slug !== 'string' || slug === '') continue
        if (typeof url !== 'string' || !/^https?:\/\//.test(url)) continue
        entries.push({
            slug,
            name: typeof name === 'string' && name !== '' ? name : slug,
            url,
        })
        if (entries.length >= MAX_ENTRIES) break
    }
    return entries
}
