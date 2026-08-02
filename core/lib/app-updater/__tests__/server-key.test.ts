import { describe, expect, it } from 'vitest'
import {
    type AsyncStorageLike,
    normalizeOrigin,
    originForServerKey,
    rememberServerOrigin,
    serverKeyFor,
} from '../server-key'

// A tiny in-memory AsyncStorage stand-in. The real module is a native shim;
// the registry only needs getItem/setItem.
function memoryStorage(initial: Record<string, string> = {}) {
    const data = { ...initial }
    const storage: AsyncStorageLike & { data: Record<string, string> } = {
        data,
        getItem: async (k: string) => data[k] ?? null,
        setItem: async (k: string, v: string) => {
            data[k] = v
        },
    }
    return storage
}

describe('serverKeyFor', () => {
    it('is a 32-char lowercase hex key', () => {
        expect(serverKeyFor('https://acme.tinycld.org')).toMatch(/^[0-9a-f]{32}$/)
    })

    it('is stable for the same address', () => {
        const a = serverKeyFor('https://acme.tinycld.org')
        const b = serverKeyFor('https://acme.tinycld.org')
        expect(a).toBe(b)
    })

    it('distinguishes different orgs on the same base domain', () => {
        // The whole point: two orgs are different servers with different builds.
        expect(serverKeyFor('https://acme.tinycld.org')).not.toBe(
            serverKeyFor('https://globex.tinycld.org')
        )
    })

    it('treats trivially different spellings of one origin as the same server', () => {
        // Otherwise the same server would occupy two bundle slots and
        // re-download on every switch between spellings.
        const canonical = serverKeyFor('https://acme.tinycld.org')
        expect(serverKeyFor('https://acme.tinycld.org/')).toBe(canonical)
        expect(serverKeyFor('  https://acme.tinycld.org  ')).toBe(canonical)
        expect(serverKeyFor('https://ACME.tinycld.org')).toBe(canonical)
        expect(serverKeyFor('https://acme.tinycld.org/some/path')).toBe(canonical)
    })

    it('distinguishes scheme and port', () => {
        expect(serverKeyFor('https://pb.example.com')).not.toBe(
            serverKeyFor('http://pb.example.com')
        )
        expect(serverKeyFor('https://pb.example.com')).not.toBe(
            serverKeyFor('https://pb.example.com:8443')
        )
    })

    it('produces a usable key for an unparseable address', () => {
        // Fail-soft: a malformed address must still yield a valid directory name
        // rather than throwing on the hot path.
        expect(serverKeyFor('not a url')).toMatch(/^[0-9a-f]{32}$/)
    })

    it('spreads similar hosts across distinct keys', () => {
        const keys = new Set(
            ['a', 'b', 'c', 'd', 'e', 'f', 'g', 'h'].map(s =>
                serverKeyFor(`https://${s}.tinycld.org`)
            )
        )
        expect(keys.size).toBe(8)
    })
})

describe('normalizeOrigin', () => {
    it('reduces a URL to scheme+host+port', () => {
        expect(normalizeOrigin('https://acme.tinycld.org/x/y?z=1')).toBe('https://acme.tinycld.org')
    })

    it('falls back to a cleaned string when unparseable', () => {
        expect(normalizeOrigin('  Not A URL/  ')).toBe('not a url')
    })
})

describe('server origin registry', () => {
    it('round-trips a key back to its origin', async () => {
        const storage = memoryStorage()
        await rememberServerOrigin(storage, 'https://acme.tinycld.org')
        const key = serverKeyFor('https://acme.tinycld.org')
        expect(await originForServerKey(storage, key)).toBe('https://acme.tinycld.org')
    })

    it('returns null for an unknown or empty key', async () => {
        const storage = memoryStorage()
        expect(await originForServerKey(storage, 'deadbeef')).toBeNull()
        expect(await originForServerKey(storage, '')).toBeNull()
    })

    it('keeps several servers at once', async () => {
        // This is what lets a rolled-back bundle be reported to the server it
        // came from after the user has switched to a different one.
        const storage = memoryStorage()
        await rememberServerOrigin(storage, 'https://acme.tinycld.org')
        await rememberServerOrigin(storage, 'https://globex.tinycld.org')
        expect(await originForServerKey(storage, serverKeyFor('https://acme.tinycld.org'))).toBe(
            'https://acme.tinycld.org'
        )
        expect(await originForServerKey(storage, serverKeyFor('https://globex.tinycld.org'))).toBe(
            'https://globex.tinycld.org'
        )
    })

    it('does not rewrite storage when the entry is unchanged', async () => {
        const storage = memoryStorage()
        await rememberServerOrigin(storage, 'https://acme.tinycld.org')
        const after = storage.data['tinycld:app-updater:origins']
        await rememberServerOrigin(storage, 'https://acme.tinycld.org')
        expect(storage.data['tinycld:app-updater:origins']).toBe(after)
    })

    it('bounds how many origins it remembers', async () => {
        const storage = memoryStorage()
        for (let i = 0; i < 40; i++) {
            await rememberServerOrigin(storage, `https://org${i}.tinycld.org`)
        }
        const map = JSON.parse(storage.data['tinycld:app-updater:origins'])
        expect(Object.keys(map).length).toBeLessThanOrEqual(32)
        // The most recent survives; the oldest is trimmed.
        expect(await originForServerKey(storage, serverKeyFor('https://org39.tinycld.org'))).toBe(
            'https://org39.tinycld.org'
        )
        expect(
            await originForServerKey(storage, serverKeyFor('https://org0.tinycld.org'))
        ).toBeNull()
    })

    it('survives corrupt stored bookkeeping', async () => {
        // Must never break the update path — worst case is one un-reported bundle.
        const storage = memoryStorage({ 'tinycld:app-updater:origins': 'not json' })
        expect(await originForServerKey(storage, 'abc')).toBeNull()
        await rememberServerOrigin(storage, 'https://acme.tinycld.org')
        expect(await originForServerKey(storage, serverKeyFor('https://acme.tinycld.org'))).toBe(
            'https://acme.tinycld.org'
        )
    })

    it('ignores a stored value that is not an object', async () => {
        const storage = memoryStorage({ 'tinycld:app-updater:origins': '["a","b"]' })
        expect(await originForServerKey(storage, 'abc')).toBeNull()
    })
})
