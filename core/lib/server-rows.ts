import { captureException } from '@tinycld/core/lib/errors'

interface AcceptingStore<T> {
    accept: (rows: readonly T[]) => Promise<void>
}

interface EvictingStore {
    evict: (ids: readonly string[]) => Promise<void>
}

/**
 * Lands a row the server returned — a custom endpoint's response — as
 * confirmed state, so the screen updates before the realtime echo arrives.
 * The server write has already succeeded, so a failure here is reported rather
 * than thrown: the echo or the next live query still delivers the row.
 */
export async function acceptServerRow<T>(
    collection: AcceptingStore<T>,
    row: T,
    context: string
): Promise<void> {
    try {
        await collection.accept([row])
    } catch (err) {
        captureException(`${context}.accept`, err)
    }
}

/**
 * Removes rows the server no longer serves to this user from every live query,
 * without waiting for a realtime delete that may never come. Reported, not
 * thrown, for the same reason as acceptServerRow.
 */
export function evictRows(
    collection: EvictingStore,
    ids: readonly string[],
    context: string
): void {
    collection.evict(ids).catch(err => captureException(`${context}.evict`, err))
}
