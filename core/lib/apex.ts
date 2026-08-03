// Telling a multi-org APEX apart from an org you can actually sign into.
//
// On a multi-org router the apex (tinycld.org) hosts no org: it answers every
// path with the org-finder page, so there is no PocketBase behind it and
// nothing to authenticate against. Each org lives one label down
// (acme.tinycld.org) as its own process and DB.
//
// The app cannot infer this from reachability — the apex is very much up. Worse,
// the router serves that finder page with HTTP 200 for /api/* too, so a probe
// that only checks res.ok reports a healthy server and the app goes on to render
// a sign-in panel that can never succeed. The discriminator therefore has to be
// the response SHAPE, not its status: a real server answers /api/org-info with
// JSON, the apex answers it with HTML.
//
// Slug handling deliberately reuses lib/org-cookie.ts rather than re-deriving
// URLs here. That module already validates a slug as exactly one DNS label and
// builds the origin from a hostname the caller supplies — the same rules the Go
// side enforces (core/server/orgcookie), and the same reason: a slug becomes the
// leftmost label of a URL we navigate to, so anything else is a planted entry.

import { orgUrlForSlug } from './org-cookie'

// Thrown when an address answers, but as a multi-org apex rather than a server.
// A distinct type (not a generic "couldn't reach") because the recovery is
// completely different: the host is fine and the user simply has to say WHICH
// org they want, so callers route to the picker instead of showing a network
// error the user cannot act on.
export class ApexServerError extends Error {
    // The apex origin itself, so the picker can label rows and build org URLs
    // from the host the user actually reached.
    readonly apexOrigin: string

    constructor(apexOrigin: string) {
        super('This address hosts organizations rather than being one.')
        this.name = 'ApexServerError'
        this.apexOrigin = apexOrigin
    }
}

/** True when a body looks like the router's org-finder page rather than a
 *  server's JSON. Checks the content type first (what the router actually
 *  sets) and falls back to sniffing a doctype, so a misconfigured proxy that
 *  serves HTML as text/plain is still caught. */
export function looksLikeApexResponse(contentType: string, body: string): boolean {
    if (contentType.toLowerCase().includes('text/html')) return true
    return /^\s*<(?:!doctype|html)\b/i.test(body)
}

/** The hostname of an address, or null when it will not parse. */
export function hostnameOf(address: string): string | null {
    try {
        return new URL(address).hostname
    } catch {
        return null
    }
}

/** Builds an org's origin from a slug typed into the picker and the apex the
 *  user reached. Returns null for anything that is not a single DNS label.
 *
 *  orgUrlForSlug derives a SIBLING of the hostname it is given (it strips the
 *  first label), which is right when the app runs on an org and wrong here: we
 *  are already at the apex, so the org is a CHILD of it. Passing a placeholder
 *  label makes the apex the parent again and keeps one implementation of the
 *  slug rules rather than a second copy that could drift from the Go side. */
export function orgUrlUnderApex(slug: string, apexHostname: string): string | null {
    return orgUrlForSlug(slug, `_.${apexHostname}`)
}

/** Whether `origin` is an org hosted under `apexHostname` — used to show only
 *  the saved servers that belong to this router, not unrelated self-hosted
 *  boxes. Requires exactly one extra label so a deeper host never passes. */
export function isOrgUnderApex(origin: string, apexHostname: string): boolean {
    const host = hostnameOf(origin)
    if (!host || host === apexHostname) return false
    if (!host.endsWith(`.${apexHostname}`)) return false
    const label = host.slice(0, -(apexHostname.length + 1))
    return label.length > 0 && !label.includes('.')
}

/** Whether a cached address turns out to be a multi-org apex rather than a
 *  server, for the recovery path: a device that connected before this was
 *  caught at admission still has the apex saved as its server.
 *
 *  ASKS THE SERVER rather than comparing hostnames. An earlier version matched
 *  the address against the configured defaultServer, which is wrong in the
 *  common case: defaultServer is the hosted address ('https://tinycld.org', or
 *  localhost in dev), and a perfectly ordinary SINGLE-TENANT server lives at
 *  exactly that hostname. Every such user was told "localhost hosts many
 *  organizations" and sent to the org picker instead of the login form. A host
 *  cannot be classified by its name — only by what it answers. */
export async function isApexAddress(address: string | null): Promise<boolean> {
    if (!address) return false
    try {
        const res = await fetch(`${address}/api/org-info`, { cache: 'no-store' })
        const body = await res.text()
        return looksLikeApexResponse(res.headers?.get?.('content-type') ?? '', body)
    } catch {
        // Unreachable is not "apex". Falling through to the normal signed-out
        // path lets the usual connectivity handling deal with it, rather than
        // stranding an offline user on a picker they cannot act on.
        return false
    }
}

/** The slug of an org origin under an apex, for labelling a picker row. */
export function slugUnderApex(origin: string, apexHostname: string): string | null {
    if (!isOrgUnderApex(origin, apexHostname)) return null
    const host = hostnameOf(origin)
    if (!host) return null
    return host.slice(0, -(apexHostname.length + 1))
}
