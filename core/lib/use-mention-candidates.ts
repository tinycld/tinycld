import { and, eq, inArray, like } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { useStore } from '@tinycld/core/lib/pocketbase'
import type { MentionSuggestion } from '@tinycld/core/ui/comments'

/** How many candidates the popover will ever render. */
export const MENTION_CANDIDATE_LIMIT = 20

/**
 * The @-mention candidate pool, searched server-side.
 *
 * A mention autocomplete is a search, not a roster read: without a predicate the
 * query fetches every user in the deployment the moment someone types `@`, and
 * holds a whole-collection realtime subscription while the editor is mounted.
 * The typed prefix, the role filter and the cap therefore all go into the
 * request.
 *
 * The predicates are written in positive form on purpose. pbtsdb compiles a
 * query's `where` to a PocketBase filter, and `not(...)` compiles to `!(...)`,
 * which PocketBase rejects outright ("invalid sign operator") — so "not a guest"
 * is spelled as the set of roles that may be mentioned, and "not disabled" as
 * `disabled = false`.
 *
 * The query is disabled (returns no rows, runs no request) when `disabled` is set
 * or nothing has been typed yet. Guests must not enumerate the roster at all, so
 * a caller with mentions switched off passes `disabled`.
 *
 * `currentUserId` is dropped in JS rather than in the `where`: it is exactly one
 * known row out of an already-capped window, and excluding it server-side would
 * need the `not()` form PocketBase refuses.
 */
export function useMentionCandidates(
    search: string,
    options?: { disabled?: boolean; currentUserId?: string }
): MentionSuggestion[] {
    const disabled = options?.disabled === true
    const currentUserId = options?.currentUserId
    const [usersCollection] = useStore('users')
    // pbtsdb's escapeValue escapes `"` but not `\`, so a term containing `\"`
    // compiles to `"\\""` — an unterminated filter literal PocketBase answers
    // with a 400. Dropping backslashes is the whole guard: a backslash has no
    // meaning to a `~` match here, so nothing searchable is lost. Same guard
    // as the audit log's search box.
    const term = search.trim().replace(/\\/g, '')

    const { data: candidates = [] } = useLiveQuery({
        query: query => {
            if (disabled || term.length === 0) return null
            return query
                .from({ u: usersCollection })
                .where(({ u }) =>
                    and(
                        inArray(u.role, ['owner', 'admin', 'member']),
                        eq(u.disabled, false),
                        // `like` compiles to `name ~ "<term>%"`. PocketBase
                        // auto-wraps a `~` operand in `%` only when it has none
                        // of its own (`wrapLikeParams`), so the explicit trailing
                        // `%` is what makes this a prefix match rather than a
                        // contains match — which is what an autocomplete wants.
                        like(u.name, `${term}%`)
                    )
                )
                .orderBy(({ u }) => u.name)
                .limit(MENTION_CANDIDATE_LIMIT)
                .select(({ u }) => ({ userId: u.id, displayName: u.name, email: u.email }))
        },
    })

    return toMentionSuggestions(candidates, currentUserId)
}

/**
 * Pure projection from the query's rows to the popover's shape. Separated so the
 * fallback chain (name → email → "Unknown") and the self-exclusion can be
 * asserted without a store.
 */
export function toMentionSuggestions(
    candidates: readonly { userId: string; displayName: string | null; email: string | null }[],
    currentUserId?: string
): MentionSuggestion[] {
    const out: MentionSuggestion[] = []
    for (const candidate of candidates) {
        if (candidate.userId === currentUserId) continue
        out.push({
            userId: candidate.userId,
            displayName: candidate.displayName || candidate.email || 'Unknown',
            secondary: candidate.email || undefined,
        })
    }
    return out
}
