// serverKeyFor derives the stable, filesystem-safe key the native app-updater
// stores a server's bundle state under.
//
// Why a key at all: each org in a multi-org deployment runs its OWN build with
// its own package set, so "the current bundle" is a property of the server the
// app is connected to. One shared slot made switching orgs re-download the
// other org's bundle on every foreground — a thrash loop, with a full JS reload
// each time.
//
// Why a hash rather than the origin itself: the value becomes a directory name
// on iOS and Android. Hashing means no origin can escape the store directory
// (`..`), collide with another via path normalization, or hit a length limit —
// and the native side additionally validates the result as pure hex before
// joining it, so a malformed key is refused rather than sanitized.
//
// This is NOT a security boundary and deliberately does not use SHA-256: every
// expo-crypto digest is async, and this must be callable synchronously from the
// address-resolution listener that runs before the first update check. Collision
// resistance against an adversary is not required — the key only has to
// distinguish the handful of servers one user connects to, and a collision would
// merely share a bundle slot between two hosts (the manifest's own SHA-256
// integrity check still governs what may load). A 128-bit FNV-1a construction is
// far beyond what that needs.

const FNV_OFFSET = 0x811c9dc5
const FNV_PRIME = 0x01000193

// fnv1a returns a 32-bit FNV-1a hash of `input`, seeded so distinct rounds
// produce independent values. Math.imul keeps the multiply in 32-bit space (a
// plain `*` would lose precision past 2^53 and collapse the avalanche).
function fnv1a(input: string, seed: number): number {
    let h = FNV_OFFSET ^ seed
    for (let i = 0; i < input.length; i++) {
        h ^= input.charCodeAt(i)
        h = Math.imul(h, FNV_PRIME)
    }
    return h >>> 0
}

// normalizeOrigin reduces an address to its scheme+host+port, so trivially
// different spellings of the same server share one bundle slot rather than
// re-downloading into two. Falls back to a trimmed, lowercased, trailing-slash-
// stripped string when the address doesn't parse as a URL.
export function normalizeOrigin(address: string): string {
    const trimmed = address.trim()
    try {
        return new URL(trimmed).origin.toLowerCase()
    } catch {
        return trimmed.toLowerCase().replace(/\/+$/, '')
    }
}

// serverKeyFor returns a 32-char lowercase hex key for a server address.
// Stable across launches and platforms — it is the identity the persisted
// bundle state is filed under, so changing this function orphans every device's
// downloaded bundles (they would re-download once, then settle).
export function serverKeyFor(address: string): string {
    const origin = normalizeOrigin(address)
    let hex = ''
    // Four independently-seeded rounds → 128 bits. Concatenating rounds (rather
    // than iterating one 32-bit state) is what makes the extra width real.
    for (let round = 0; round < 4; round++) {
        hex += fnv1a(origin, round).toString(16).padStart(8, '0')
    }
    return hex
}

// The key→origin directory, persisted so a bundle rolled back at native boot can
// still be reported to the server it CAME FROM. The rollback happens before any
// JS runs and the marker only carries the key (a one-way hash), so without this
// the report would have to go to whichever server happens to be active — after a
// switch, the wrong one: it would blocklist an id on a server that never served
// it while the actually-bad bundle kept being advertised by its own.
//
// AsyncStorage rather than the native store: this is pure JS bookkeeping, and
// keeping it out of the pre-bridge loader keeps that path as small as possible.
const ORIGINS_KEY = 'tinycld:app-updater:origins'

// Bounded so a device that connects to many servers over its lifetime cannot
// grow this without limit. Far above any real user's server count.
const MAX_REMEMBERED_ORIGINS = 32

type OriginMap = Record<string, string>

async function readOrigins(storage: AsyncStorageLike): Promise<OriginMap> {
    try {
        const raw = await storage.getItem(ORIGINS_KEY)
        if (!raw) return {}
        const parsed: unknown = JSON.parse(raw)
        if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return {}
        return parsed as OriginMap
    } catch {
        // Corrupt or unreadable bookkeeping must never break the update path —
        // the worst case is one un-reported bad bundle.
        return {}
    }
}

// Minimal shape of the AsyncStorage surface used here, so tests can inject.
export interface AsyncStorageLike {
    getItem(key: string): Promise<string | null>
    setItem(key: string, value: string): Promise<void>
}

// rememberServerOrigin records key→origin. Idempotent; trims oldest-first when
// the map exceeds MAX_REMEMBERED_ORIGINS (insertion order is preserved by JSON
// round-trip for string keys that aren't array indices, which hex keys are not).
export async function rememberServerOrigin(
    storage: AsyncStorageLike,
    address: string
): Promise<void> {
    const key = serverKeyFor(address)
    const origin = normalizeOrigin(address)
    const map = await readOrigins(storage)
    if (map[key] === origin) return
    map[key] = origin
    const keys = Object.keys(map)
    if (keys.length > MAX_REMEMBERED_ORIGINS) {
        for (const stale of keys.slice(0, keys.length - MAX_REMEMBERED_ORIGINS)) {
            delete map[stale]
        }
    }
    try {
        await storage.setItem(ORIGINS_KEY, JSON.stringify(map))
    } catch {
        // Best-effort, as above.
    }
}

// originForServerKey resolves a remembered key back to its origin, or null.
export async function originForServerKey(
    storage: AsyncStorageLike,
    serverKey: string
): Promise<string | null> {
    if (!serverKey) return null
    const map = await readOrigins(storage)
    return map[serverKey] ?? null
}
