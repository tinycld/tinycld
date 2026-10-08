interface SyncingStore {
    status: string
    reload: () => Promise<unknown>
}

/**
 * Reload every store that is already syncing: each refetches its live
 * queries' subsets. An idle or torn-down store has nothing live to reload.
 *
 * Used where the server's state may have moved on without a realtime event:
 * rows loaded before sign-in were fetched anonymously (a list rule hides every
 * row the signed-in user owns from an anonymous reader, so they are wrong, not
 * merely stale — `user_preferences` was the case that bit), and an app that
 * comes back to the foreground or back online missed whatever happened while
 * it was away.
 */
export async function reloadLoadedStores(stores: Iterable<SyncingStore>): Promise<void> {
    const syncing = [...stores].filter(
        store => store.status !== 'idle' && store.status !== 'cleaned-up'
    )
    await Promise.all(syncing.map(store => store.reload()))
}
