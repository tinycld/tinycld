// Cache-aware imperative read of a pbtsdb collection, for the rare non-React
// contexts where `useLiveQuery` can't be used (inside a `mutationFn`/`queryFn`
// callback, an imperative util, a Zustand action — anywhere hooks are illegal).
//
// Why not a raw `pb.collection(name).getFullList()/getFirstListItem()`: that
// ALWAYS hits the network and ignores records the pbtsdb store may already hold
// (loaded by a mounted `useLiveQuery`, a prior read, or `preload()`). A pbtsdb
// collection exposes `toArrayWhenReady()`, which resolves from the optimistic
// store — preloading once if the collection isn't loaded yet, and reusing the
// in-memory rows otherwise. So this shares the same data a `useLiveQuery` sees,
// stays consistent with optimistic mutations, and avoids a redundant round-trip.
//
// Callers filter/find on the returned array in JS. NOTE what that array is:
// every collection is on-demand (pbtsdb 0.10), so the store holds only the rows
// some live query has already asked for, plus whatever `toArrayWhenReady()`'s
// own preload brought in. It is NOT the whole collection. So a predicate here
// answers "among the rows we hold", which is right for confirming a row the
// caller just wrote or saw, and wrong for "does any row match" — that needs the
// matching live query (or a direct PocketBase read) to have run first.

// Structural type: anything that can yield its rows once the store is ready.
// pbtsdb/TanStack-DB collections satisfy this; tests pass a lightweight fake.
interface ReadableCollection<T> {
    toArrayWhenReady(): Promise<T[]>
}

/**
 * Read all rows of a collection from the pbtsdb store (cache-aware), optionally
 * filtered by a predicate. Prefer this over a raw `pb.collection(...).getXxx()`
 * in any non-React context.
 */
export async function readCollectionCached<T>(
    collection: ReadableCollection<T>,
    predicate?: (row: T) => boolean
): Promise<T[]> {
    const rows = await collection.toArrayWhenReady()
    return predicate ? rows.filter(predicate) : rows
}

/**
 * Cache-aware equivalent of `pb.collection(...).getFirstListItem(...)`: returns
 * the first row matching `predicate`, or `undefined` if none. Reads from the
 * pbtsdb store rather than the network.
 */
export async function findCollectionCached<T>(
    collection: ReadableCollection<T>,
    predicate: (row: T) => boolean
): Promise<T | undefined> {
    const rows = await collection.toArrayWhenReady()
    return rows.find(predicate)
}
