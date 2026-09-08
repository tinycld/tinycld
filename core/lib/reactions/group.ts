// Reaction rows -> the per-target chip model.
//
// Generic over the row type and the grouping key, so one fold serves any
// package's reactions table — a package keys its rows by whatever it hangs
// them off, and may have several such tables. Core never learns what they are.
//
// Pure and side-effect free — no React, no collections, no PocketBase. The
// package owns the query; this owns the shape its UI consumes.

/** The minimum a reaction row must carry. A package's row will have more. */
export interface ReactionRow {
    id: string
    user: string
    emoji: string
}

export interface ReactionGroup {
    emoji: string
    count: number
    /** The caller's own row, so a toggle can delete without a second lookup. */
    ownId: string | null
    /**
     * Who reacted, in arrival order — ids, not names.
     *
     * Ids keep this function pure and testable; resolving them to people is
     * the caller's job, because only the package knows which user store to
     * join and what to show for a row the viewer cannot read.
     */
    userIds: string[]
}

/**
 * Rows -> one group per (target, emoji), each target's groups ordered by count
 * descending, ties broken by first appearance.
 *
 * There is no fixed palette to order by — the picker offers the whole emoji
 * set — and arrival order alone would let a chip jump position as other people
 * react. Ties break on first appearance so equal counts stay put.
 *
 * `userId` may be empty for an anonymous reader (a public board), in which
 * case no group is ever marked as the caller's own.
 */
export function groupReactions<TRow extends ReactionRow>(
    rows: readonly TRow[],
    userId: string,
    keyOf: (row: TRow) => string
): Map<string, ReactionGroup[]> {
    const byTarget = new Map<string, Map<string, ReactionGroup & { seq: number }>>()

    let seq = 0
    for (const row of rows) {
        const target = keyOf(row)
        let groups = byTarget.get(target)
        if (!groups) {
            groups = new Map()
            byTarget.set(target, groups)
        }
        let group = groups.get(row.emoji)
        if (!group) {
            group = { emoji: row.emoji, count: 0, ownId: null, userIds: [], seq: seq++ }
            groups.set(row.emoji, group)
        }
        group.count += 1
        group.userIds.push(row.user)
        if (userId !== '' && row.user === userId) group.ownId = row.id
    }

    const out = new Map<string, ReactionGroup[]>()
    for (const [target, groups] of byTarget) {
        out.set(
            target,
            [...groups.values()]
                .sort((a, b) => b.count - a.count || a.seq - b.seq)
                .map(({ seq: _seq, ...group }) => group)
        )
    }
    return out
}

/**
 * A stable per-emoji key for test ids and React keys: the unified codepoints.
 *
 * Not the glyph — a raw emoji in a selector is painful to type and, for a ZWJ
 * sequence, ambiguous to read. Synchronous and total, so a chip never waits on
 * the lazily-loaded emoji table.
 */
export function reactionKey(emoji: string): string {
    return [...emoji].map(char => (char.codePointAt(0) ?? 0).toString(16)).join('-')
}
