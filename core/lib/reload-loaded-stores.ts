interface SyncingStore {
    status: string
    utils: { refetch: () => Promise<unknown> }
}

/**
 * Fetch every store that is already syncing again.
 *
 * Rows loaded before sign-in were fetched anonymously, and a list rule hides
 * from an anonymous reader every row the signed-in user owns — so they are not
 * merely stale but wrong. `user_preferences` is the case that bit: the theme
 * hook at the root syncs it on the sign-in screen, its empty snapshot survived
 * login, and every later preference write became an insert the unique index
 * refused (the emoji picker's skin tone silently reverted). A store that has
 * not started syncing fetches under the new identity on its own.
 */
export async function refetchLoadedStores(stores: Iterable<SyncingStore>): Promise<void> {
    const syncing = [...stores].filter(
        store => store.status !== 'idle' && store.status !== 'cleaned-up'
    )
    await Promise.all(syncing.map(store => store.utils.refetch()))
}
