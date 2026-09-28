import { inArray } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { parseMentions } from '@tinycld/core/lib/comments/mentions'
import { useStore } from '@tinycld/core/lib/pocketbase'

/**
 * The `userId → displayName` map needed to render `[[@id]]` tokens in stored
 * comment bodies as `@Alice`.
 *
 * This exists because the mention picker's candidate list is no longer the whole
 * roster. It used to double as the name source for read mode, which worked only
 * by accident: the picker fetched every user, so every id a body could mention
 * was already in hand. Now the picker is a bounded search keyed on what the user
 * typed, so read mode has to look its ids up itself — otherwise a body renders
 * the raw `[[@id]]` token.
 *
 * The lookup is bounded by the ids the bodies actually mention, which is the
 * natural scope: a thread names a handful of people, not an organization. That
 * also makes it cheap on a deployment of any size, and it resolves a mention of
 * someone the picker would no longer offer — a deactivated account, or a user
 * whose name no longer prefix-matches.
 *
 * The query is disabled (returns `null`, issues no request) when no body mentions
 * anyone. That is not an optimization but a correctness requirement: pbtsdb
 * compiles `inArray(field, [])` by mapping the values to `field = <v>` clauses
 * and joining them, so an empty array yields `undefined` — a malformed filter the
 * server rejects.
 */
export function useMentionNames(bodies: readonly string[]): Map<string, string> {
    const [usersCollection] = useStore('users')
    // Sorted so the id list — and therefore the query's identity — does not
    // change when the same people are mentioned in a different order.
    const mentionedIds = [
        ...new Set(bodies.flatMap(body => parseMentions(body).map(m => m.userId))),
    ]
        .sort()
        .join(',')

    const { data: rows = [] } = useLiveQuery({
        query: query => {
            if (!mentionedIds) return null
            return query
                .from({ u: usersCollection })
                .where(({ u }) => inArray(u.id, mentionedIds.split(',')))
                .select(({ u }) => ({ id: u.id, name: u.name, email: u.email }))
        },
    })

    return new Map(rows.map(row => [row.id, row.name || row.email || row.id]))
}
