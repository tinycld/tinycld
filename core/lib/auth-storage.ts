import AsyncStorage from '@react-native-async-storage/async-storage'
import { serverKeyFor } from './app-updater/server-key'
import { normalizeAddress } from './server-address'

// The auth blob used to live under one flat key, so two servers could not hold
// tokens at once and connecting elsewhere silently signed you out of the first.
// It is now keyed by server: `pb_auth:<serverKey>`, reusing the same identity
// the bundle store files under, so one server has one key everywhere.
export const LEGACY_AUTH_KEY = 'pb_auth'

// Normalizes before hashing: serverKeyFor falls back to hashing the RAW string
// when the address has no scheme (its URL parse throws), so a bare
// `acme.tinycld.org` would key a different blob than the stored
// `https://acme.tinycld.org` — the same session filed under two keys.
export function authKeyFor(address: string): string {
    return `${LEGACY_AUTH_KEY}:${serverKeyFor(normalizeAddress(address))}`
}

// Minimal storage surface, so tests inject a map instead of AsyncStorage.
export interface AuthStorageLike {
    getItem(key: string): Promise<string | null>
    setItem(key: string, value: string): Promise<void>
    removeItem(key: string): Promise<void>
}

// readAuthBlob returns the stored auth for one server, migrating a legacy flat
// blob into that server's scoped key on first run.
//
// The migration is the sharp edge of this whole feature: get it wrong and every
// existing user is signed out on upgrade. Two rules make it safe:
//
//  1. The scoped key WINS when present. A legacy blob may linger (another
//     server's migration ran first and left it, or a partial write); it must
//     never clobber a real scoped session.
//  2. The legacy blob is only adopted by the server that is ACTIVE at the time
//     of migration. It came from a single-server install, so the active server
//     is by definition the one it belongs to. Adopting it for any other server
//     would hand server A's token to server B.
//
// Applies on every platform, including web. Web's localStorage is already
// origin-partitioned by the browser, so its key is stable and the migration is a
// one-time rename with no behavioral change — one storage contract everywhere
// beats two, and web then costs nothing extra to reason about.
export async function readAuthBlob(
    address: string,
    storage: AuthStorageLike = AsyncStorage
): Promise<string | null> {
    const scopedKey = authKeyFor(address)

    const scoped = await storage.getItem(scopedKey)
    if (scoped) return scoped

    const legacy = await storage.getItem(LEGACY_AUTH_KEY)
    if (!legacy) return null

    // Move rather than copy: leaving the flat key behind would let a later
    // switch to a different server adopt this same token as its own.
    await storage.setItem(scopedKey, legacy)
    await storage.removeItem(LEGACY_AUTH_KEY)
    return legacy
}

export async function writeAuthBlob(
    address: string,
    serialized: string,
    storage: AuthStorageLike = AsyncStorage
): Promise<void> {
    await storage.setItem(authKeyFor(address), serialized)
}

export async function clearAuthBlob(
    address: string,
    storage: AuthStorageLike = AsyncStorage
): Promise<void> {
    await storage.removeItem(authKeyFor(address))
    // Also drop any legacy blob: this server is the only one that could have
    // adopted it, so leaving it would resurrect a signed-out session on the
    // next launch (readAuthBlob would find it and migrate it back in).
    await storage.removeItem(LEGACY_AUTH_KEY)
}

// Parsed shape of a stored blob. `model` is the legacy PocketBase field name and
// still appears in blobs written by older versions, so both are tolerated.
export interface StoredAuth {
    token?: string
    record?: unknown
    model?: unknown
}

export function parseAuthBlob(raw: string): StoredAuth | null {
    try {
        const parsed = JSON.parse(raw) as StoredAuth
        if (!parsed || typeof parsed !== 'object') return null
        return parsed
    } catch {
        return null
    }
}
