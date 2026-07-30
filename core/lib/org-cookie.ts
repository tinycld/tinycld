// The cross-org switcher cookie. Each router-managed tenant upserts its own
// {slug, name} entry at login, scoped to the parent domain
// (Domain=.<MT_BASE_DOMAIN>), so the browser accumulates the orgs this user
// has actually signed into. It is a NAVIGATION HINT, not an authorization
// claim — it is client-writable, and every target org still authenticates the
// user itself. The server-side writer is serve-org (multi-org repo,
// internal/orgcookie); the two must agree on this shape.
//
// Entries deliberately carry NO URL. The cookie is writable by JS on any
// sibling tenant, so a stored URL would be an attacker-controlled navigation
// target — one hostile org could point another org's switcher entry at a
// login clone. Each org's URL is instead derived from its slug and the parent
// domain of the origin this app is running on (orgUrlForSlug), data the
// cookie cannot influence.

export const ORGS_COOKIE_NAME = 'tinycld_orgs'

export interface CrossOrgEntry {
    slug: string
    name: string
}

// Entries beyond this are dropped (oldest first) — matches the server-side
// writer's cap so the cookie can't grow unboundedly.
const MAX_ENTRIES = 20

// Exactly one lowercase DNS label — what the router's subdomain dispatch
// produces. The slug becomes the leftmost label of a URL this app navigates
// to, so anything else ("evil.example/x", "a.b") is a planted entry.
const SLUG_PATTERN = /^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/

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
        const { slug, name } = item as Record<string, unknown>
        if (typeof slug !== 'string' || !SLUG_PATTERN.test(slug)) continue
        entries.push({
            slug,
            name: typeof name === 'string' && name !== '' ? name : slug,
        })
        if (entries.length >= MAX_ENTRIES) break
    }
    return entries
}

/** Derives an org's origin from its slug and the hostname this app is served
 *  from: https://<slug>.<parent-of-currentHostname>. Null when the slug is
 *  not a single DNS label or the hostname has no parent domain (a bare host
 *  like localhost cannot have sibling orgs). */
export function orgUrlForSlug(slug: string, currentHostname: string): string | null {
    if (!SLUG_PATTERN.test(slug)) return null
    const dot = currentHostname.indexOf('.')
    if (dot < 0 || dot === currentHostname.length - 1) return null
    return `https://${slug}${currentHostname.slice(dot)}`
}

/** Derives the apex origin (the router's org-finder page) from the hostname
 *  this app is served from: https://<parent-of-currentHostname>. The parent
 *  must itself contain a dot — the "parent" of a bare registrable domain is a
 *  TLD, not an apex we host — so a standalone deployment on its own domain
 *  gets null and no finder link. */
export function apexUrlForHost(currentHostname: string): string | null {
    const dot = currentHostname.indexOf('.')
    if (dot < 0) return null
    const parent = currentHostname.slice(dot + 1)
    if (!parent.includes('.') || parent.endsWith('.')) return null
    return `https://${parent}`
}
