import { beforeEach, describe, expect, it } from 'vitest'
import {
    type AuthStorageLike,
    authKeyFor,
    clearAuthBlob,
    LEGACY_AUTH_KEY,
    parseAuthBlob,
    readAuthBlob,
    writeAuthBlob,
} from '../auth-storage'

function memoryStorage(initial: Record<string, string> = {}) {
    const map = new Map(Object.entries(initial))
    const storage: AuthStorageLike & { snapshot: () => Record<string, string> } = {
        getItem: async key => map.get(key) ?? null,
        setItem: async (key, value) => {
            map.set(key, value)
        },
        removeItem: async key => {
            map.delete(key)
        },
        snapshot: () => Object.fromEntries(map),
    }
    return storage
}

const SERVER_A = 'https://a.example.com'
const SERVER_B = 'https://b.example.com'

const blobA = JSON.stringify({ token: 'token-a', record: { id: 'user-a' } })
const blobB = JSON.stringify({ token: 'token-b', record: { id: 'user-b' } })

describe('auth-storage', () => {
    let storage: ReturnType<typeof memoryStorage>

    beforeEach(() => {
        storage = memoryStorage()
    })

    it('keys the blob by server, so distinct servers get distinct keys', () => {
        expect(authKeyFor(SERVER_A)).not.toBe(authKeyFor(SERVER_B))
        expect(authKeyFor(SERVER_A)).toMatch(/^pb_auth:[0-9a-f]{32}$/)
    })

    // serverKeyFor hashes the RAW string when the address has no scheme (its URL
    // parse throws), so without normalizing first a bare hostname files the same
    // session under a second key — and the user appears signed out.
    it('treats spelling variants of one server as the same key', () => {
        const canonical = authKeyFor('https://acme.tinycld.org')
        expect(authKeyFor('acme.tinycld.org')).toBe(canonical)
        expect(authKeyFor('https://acme.tinycld.org/')).toBe(canonical)
        expect(authKeyFor('https://ACME.tinycld.org')).toBe(canonical)
    })

    it('reads back a blob written under a differently-spelled address', async () => {
        await writeAuthBlob('https://acme.tinycld.org', blobA, storage)
        expect(await readAuthBlob('acme.tinycld.org', storage)).toBe(blobA)
    })

    // The actual invariant of step 1b: a second server does not evict the first.
    it('holds two servers tokens at once, and reads back the right one', async () => {
        await writeAuthBlob(SERVER_A, blobA, storage)
        await writeAuthBlob(SERVER_B, blobB, storage)

        expect(await readAuthBlob(SERVER_A, storage)).toBe(blobA)
        expect(await readAuthBlob(SERVER_B, storage)).toBe(blobB)
    })

    it('does not let hydrating B read As blob', async () => {
        await writeAuthBlob(SERVER_A, blobA, storage)
        expect(await readAuthBlob(SERVER_B, storage)).toBeNull()
    })

    describe('legacy migration', () => {
        it('adopts a legacy flat blob for the active server and removes the flat key', async () => {
            storage = memoryStorage({ [LEGACY_AUTH_KEY]: blobA })

            expect(await readAuthBlob(SERVER_A, storage)).toBe(blobA)

            const after = storage.snapshot()
            expect(after[authKeyFor(SERVER_A)]).toBe(blobA)
            expect(after[LEGACY_AUTH_KEY]).toBeUndefined()
        })

        // Without the move, a later switch would adopt the same token as its own.
        it('does not hand a migrated token to a second server', async () => {
            storage = memoryStorage({ [LEGACY_AUTH_KEY]: blobA })
            await readAuthBlob(SERVER_A, storage)

            expect(await readAuthBlob(SERVER_B, storage)).toBeNull()
        })

        it('prefers an existing scoped key over a lingering legacy blob', async () => {
            storage = memoryStorage({
                [LEGACY_AUTH_KEY]: blobB,
                [authKeyFor(SERVER_A)]: blobA,
            })

            expect(await readAuthBlob(SERVER_A, storage)).toBe(blobA)
            // The legacy blob is left alone — it is not this server's to consume.
            expect(storage.snapshot()[LEGACY_AUTH_KEY]).toBe(blobB)
        })

        it('migrates a legacy blob using the pre-record `model` field', async () => {
            const legacyShape = JSON.stringify({ token: 't', model: { id: 'user-a' } })
            storage = memoryStorage({ [LEGACY_AUTH_KEY]: legacyShape })

            const migrated = await readAuthBlob(SERVER_A, storage)
            expect(migrated).toBe(legacyShape)

            const parsed = parseAuthBlob(migrated as string)
            expect(parsed?.record ?? parsed?.model).toEqual({ id: 'user-a' })
        })
    })

    it('clearing a server drops its key and any legacy blob', async () => {
        storage = memoryStorage({ [LEGACY_AUTH_KEY]: blobA })
        await writeAuthBlob(SERVER_A, blobA, storage)
        await writeAuthBlob(SERVER_B, blobB, storage)

        await clearAuthBlob(SERVER_A, storage)

        const after = storage.snapshot()
        expect(after[authKeyFor(SERVER_A)]).toBeUndefined()
        expect(after[LEGACY_AUTH_KEY]).toBeUndefined()
        // The other server is untouched.
        expect(after[authKeyFor(SERVER_B)]).toBe(blobB)
    })

    it('parseAuthBlob returns null on corrupt json rather than throwing', () => {
        expect(parseAuthBlob('{not json')).toBeNull()
    })
})
