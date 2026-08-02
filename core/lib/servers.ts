import AsyncStorage from '@react-native-async-storage/async-storage'
import { Platform } from 'react-native'
import { normalizeOrigin, serverKeyFor } from './app-updater/server-key'
import { normalizeAddress, readCached, writeCached } from './server-address'

// The ordered list of servers the user has connected to on this device. The
// ACTIVE one is not stored here — it stays in `tinycld:server:app` (see
// server-address.ts), because the gate, readCached() and the connectivity
// detector all read that key today and keep working untouched.
//
// A "server" here is just an origin. That is deliberately the same object for
// both deployment shapes: a self-hosted box is one origin, and each org on the
// hosted multi-org router is its own subdomain — hence its own origin, its own
// process, its own DB. serverKeyFor normalizes to scheme+host+port, so two orgs
// on the router get distinct keys exactly as two self-hosted boxes do, and
// nothing below needs to know which shape it is looking at.
const SERVERS_KEY = 'tinycld:servers'

// Bounded so a device that connects to many servers over its lifetime cannot
// grow this without limit. Well above any plausible real-world server count,
// and below the app-updater's 32-entry origin map so the bundle bookkeeping
// always covers every saved server.
export const MAX_SAVED_SERVERS = 10

export interface SavedServer {
    // Normalized origin (scheme+host+port). The identity for everything else:
    // serverKeyFor(origin) keys the auth blob and the bundle store.
    origin: string
    // Display label. probe() only checks /api/health and returns void, so there
    // is no server-provided name to show — we label with the hostname, which is
    // also what the user typed. Good for both shapes: self-hosted users
    // recognize their host, and org subdomains are already named after the org.
    label: string
    addedAt: number
}

// keyOf is the ONLY way this module derives a server key from user-supplied
// input. serverKeyFor alone is not enough: given a bare `a.example.com` its
// URL parse throws and it falls back to hashing the raw string, which is a
// DIFFERENT key than the stored `https://a.example.com` — so a lookup by
// hostname would silently miss. normalizeAddress first makes every spelling of
// one server converge on one key.
function keyOf(address: string): string {
    return serverKeyFor(normalizeAddress(address))
}

function labelFor(origin: string): string {
    try {
        return new URL(origin).host
    } catch {
        return origin
    }
}

function isSavedServer(value: unknown): value is SavedServer {
    if (!value || typeof value !== 'object') return false
    const candidate = value as Record<string, unknown>
    return typeof candidate.origin === 'string' && candidate.origin.length > 0
}

export async function readServers(): Promise<SavedServer[]> {
    try {
        const raw = await AsyncStorage.getItem(SERVERS_KEY)
        if (!raw) return []
        const parsed: unknown = JSON.parse(raw)
        if (!Array.isArray(parsed)) return []
        // Tolerate partially-written entries rather than dropping the whole
        // list: losing the list would strand the user's other servers.
        return parsed.filter(isSavedServer).map(entry => ({
            origin: entry.origin,
            label: entry.label || labelFor(entry.origin),
            addedAt: typeof entry.addedAt === 'number' ? entry.addedAt : 0,
        }))
    } catch {
        return []
    }
}

async function writeServers(servers: SavedServer[]): Promise<void> {
    await AsyncStorage.setItem(SERVERS_KEY, JSON.stringify(servers))
}

// addServer appends an origin to the list, de-duplicating by serverKeyFor rather
// than raw string equality — so `https://acme.tinycld.org/`, `acme.tinycld.org`
// and `https://ACME.tinycld.org` are one entry, not three. Returns the
// normalized origin so callers use the same spelling everywhere.
//
// Routes the input through normalizeAddress so a bare hostname gains `https://`
// exactly as it does on the connect screen — there must be one such rule.
export async function addServer(address: string): Promise<string> {
    const origin = normalizeAddress(address)
    const key = keyOf(origin)
    const servers = await readServers()

    const existing = servers.find(s => keyOf(s.origin) === key)
    if (existing) return existing.origin

    const next = [
        ...servers,
        { origin, label: labelFor(origin), addedAt: Date.now() } satisfies SavedServer,
    ]
    await writeServers(await trimToCap(next))
    return origin
}

// trimToCap drops oldest-first, but never the ACTIVE server. A user can sit on
// one server for a long time while adding others, which makes the active entry
// the oldest — and evicting it would leave `tinycld:server:app` pointing at a
// server absent from its own list: the switcher would omit the server you are
// actually on, and offer only ones you are not.
async function trimToCap(servers: SavedServer[]): Promise<SavedServer[]> {
    if (servers.length <= MAX_SAVED_SERVERS) return servers

    const active = await readCached()
    const activeKey = active ? keyOf(active) : null
    // Only reserve a slot when the active server is actually IN this list —
    // otherwise the reservation would shrink the cap for no one.
    const activeIsListed = !!activeKey && servers.some(s => keyOf(s.origin) === activeKey)
    const evictable = servers.filter(s => keyOf(s.origin) !== activeKey)
    const kept = evictable.slice(-(MAX_SAVED_SERVERS - (activeIsListed ? 1 : 0)))

    // Rebuild in the original order so "oldest" stays meaningful next time.
    const keptKeys = new Set(kept.map(s => keyOf(s.origin)))
    return servers.filter(s => keyOf(s.origin) === activeKey || keptKeys.has(keyOf(s.origin)))
}

export async function removeServer(origin: string): Promise<SavedServer[]> {
    const key = keyOf(origin)
    const remaining = (await readServers()).filter(s => keyOf(s.origin) !== key)
    await writeServers(remaining)
    return remaining
}

// setActiveServer is the ONLY sanctioned writer of the active-server pointer.
//
// It writes both halves — the list entry and `tinycld:server:app` — because they
// must not diverge. Three call sites used to write the cached address directly
// (app/connect.tsx, app/connect.web.tsx, app/p/demo.tsx); a raw writeCached()
// now sets an active server WITHOUT a matching list entry, so e.g. a demo
// deep-link would produce an active server that never appears in the switcher.
export async function setActiveServer(address: string): Promise<string> {
    const origin = await addServer(address)
    await writeCached(origin)
    return origin
}

// isSavedServersSupported gates every new surface. Web resolves same-origin via
// window.location (resolveEnvAddress short-circuits before any cache read) and
// already has its own cookie-based org switcher, so a saved-server list there
// would be dead UI over an address that cannot change.
export function isSavedServersSupported(): boolean {
    return Platform.OS !== 'web'
}

// Compares two addresses as SERVERS, tolerating spelling differences on either
// side — including a bare hostname against a stored origin.
export function isSameServer(a: string, b: string): boolean {
    return keyOf(a) === keyOf(b)
}

export { normalizeOrigin, serverKeyFor }
