import { describe, expect, it, vi } from 'vitest'

// preloadStores' job changed shape with pbtsdb 0.10: `preload()` on an
// on-demand collection fetches nothing, so warming a store now means running
// the same live queries the app is about to run, and clearStores has to tear
// those queries down BEFORE their source collections.
//
// The pbtsdb factory is replaced with local-only TanStack DB collections: the
// real module graph, real createLiveQueryCollection, no network. Each fake
// records what it was asked to sync so the warmed predicates are observable.
const h = vi.hoisted(() => ({
    authRecord: null as { id: string } | null,
    /** Collections whose sync has been started, in start order. */
    started: [] as string[],
    /** Collection names still live (not cleaned up), by cleanup order. */
    cleanupOrder: [] as string[],
}))

vi.mock('pocketbase', async () => {
    const actual = await vi.importActual<typeof import('pocketbase')>('pocketbase')
    // The real client with only the authenticated record swapped. pocketbase.ts
    // exercises a fair amount of the SDK at module scope (autoCancellation,
    // realtime, authStore.onChange), and a hand-rolled double just
    // re-implements it badly; defineProperty is the smallest honest seam.
    class TestPB extends actual.default {
        constructor(...args: ConstructorParameters<typeof actual.default>) {
            super(...args)
            Object.defineProperty(this.authStore, 'record', { get: () => h.authRecord })
            Object.defineProperty(this.authStore, 'isValid', { get: () => !!h.authRecord })
            Object.defineProperty(this.authStore, 'token', {
                get: () => (h.authRecord ? 'test-token' : ''),
            })
        }
    }
    return { ...actual, default: TestPB }
})

vi.mock('@tanstack/db', async () => {
    const actual = await vi.importActual<typeof import('@tanstack/db')>('@tanstack/db')
    return {
        ...actual,
        // Records each warmed live query's teardown in the SAME list as the
        // source collections' so their relative order is assertable — the
        // property clearStores exists to preserve.
        createLiveQueryCollection: (
            ...args: Parameters<typeof actual.createLiveQueryCollection>
        ) => {
            const lq = actual.createLiveQueryCollection(...args)
            const cleanup = lq.cleanup.bind(lq)
            lq.cleanup = async () => {
                h.cleanupOrder.push('live-query')
                return cleanup()
            }
            return lq
        },
    }
})

vi.mock('pbtsdb', async () => {
    const actual = await vi.importActual<typeof import('pbtsdb')>('pbtsdb')
    const { createCollection: createTsdbCollection, localOnlyCollectionOptions } =
        await vi.importActual<typeof import('@tanstack/db')>('@tanstack/db')
    return {
        ...actual,
        createCollection: () => (name: string) => {
            const collection = createTsdbCollection(
                localOnlyCollectionOptions({
                    id: `fake-${name}`,
                    getKey: (row: { id: string }) => row.id,
                })
            )
            const started = collection.startSyncImmediate.bind(collection)
            collection.startSyncImmediate = () => {
                h.started.push(name)
                started()
            }
            const cleanup = collection.cleanup.bind(collection)
            collection.cleanup = async () => {
                h.cleanupOrder.push(name)
                return cleanup()
            }
            // pbtsdb's query-backed collections expose a refetch; the
            // local-only options do not, and refetchLoadedStores calls it.
            return Object.assign(collection, {
                collectionName: name,
                utils: { ...collection.utils, refetch: async () => {} },
            })
        },
    }
})

async function loadModule() {
    vi.resetModules()
    h.started.length = 0
    h.cleanupOrder.length = 0
    const { setResolvedAddress } = await import('@tinycld/core/lib/server-address')
    setResolvedAddress('http://localhost:8090')
    return await import('@tinycld/core/lib/pocketbase')
}

describe('preloadStores', () => {
    it('warms nothing when there is no authenticated user', async () => {
        h.authRecord = null
        const { preloadStores, clearStores } = await loadModule()

        await preloadStores()
        // Every store is on-demand, so with no user id there is no predicate to
        // warm — and warming with a blank id would fetch rows whose FK is empty.
        await clearStores()
        expect(h.cleanupOrder).not.toContain('live-query')
    })

    it('warms the live queries the first screens will run', async () => {
        h.authRecord = { id: 'u1' }
        const { preloadStores, stores, clearStores } = await loadModule()

        await preloadStores()

        // The three collections whose keys the bootstrap hooks read: the
        // current user (useCurrentRole), their package overrides
        // (useAccessiblePackages) and the registry (usePackages).
        for (const name of ['users', 'org_pkg_access', 'pkg_registry'] as const) {
            expect(stores[name].status, `${name} was never warmed`).not.toBe('idle')
        }
        await clearStores()
    })

    it('tears the warmed queries down before their source collections', async () => {
        h.authRecord = { id: 'u1' }
        const { preloadStores, clearStores, stores } = await loadModule()

        await preloadStores()
        await clearStores()

        // A live query over a cleaned-up source throws on its next change, so
        // every warmed query must be gone before the first store is.
        const lastQuery = h.cleanupOrder.lastIndexOf('live-query')
        const firstStore = h.cleanupOrder.findIndex(n => n !== 'live-query')
        expect(lastQuery, 'no warmed query was torn down').toBeGreaterThanOrEqual(0)
        expect(firstStore, 'no store was torn down').toBeGreaterThanOrEqual(0)
        expect(lastQuery).toBeLessThan(firstStore)

        for (const store of Object.values(stores)) {
            expect(store.status).toBe('cleaned-up')
        }
    })

    it('is idempotent — a second clear does not throw', async () => {
        h.authRecord = { id: 'u1' }
        const { preloadStores, clearStores } = await loadModule()

        await preloadStores()
        await clearStores()
        await expect(clearStores()).resolves.toBeUndefined()
    })
})
