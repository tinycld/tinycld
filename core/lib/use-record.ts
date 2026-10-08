import { type Collection, eq, type UtilsRecord } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'

/**
 * One record by id, live: the way to render a record you hold only the id of.
 *
 * A `collection.get(id)` in a component is a one-time read: it does not
 * re-render when the row changes, and the row can leave the store while the
 * component still shows it — a row is kept only while some live query holds it.
 * This hook's live query holds the row for as long as the component is
 * mounted. When the row is already in the store (another query or a parent's
 * relation filed it) pbtsdb serves the id lookup from the store with no
 * request.
 *
 * Disabled — no query, `record` undefined, not loading — while `id` is empty,
 * because an empty id is a real value that matches nothing.
 *
 * ```ts
 * const [usersCollection] = useStore('users')
 * const { record: author } = useRecord(usersCollection, comment.author)
 * ```
 */
export function useRecord<
    TRow extends { id: string },
    TUtils extends UtilsRecord,
    TInsert extends object,
>(
    collection: Collection<TRow, string | number, TUtils, never, TInsert>,
    id: string | null | undefined
): { record: TRow | undefined; isLoading: boolean } {
    const { data, isLoading } = useLiveQuery({
        query: q =>
            id
                ? q
                      .from({ r: collection })
                      .where(({ r }) => eq(r.id, id))
                      .findOne()
                : null,
    })
    return { record: id ? data : undefined, isLoading: id ? isLoading : false }
}
