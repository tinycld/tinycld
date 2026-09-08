// Emoji search, adapted from emoji-picker-react's alphaNumericEmojiIndex and
// the pure parts of its useFilter — without the DOM scrolling, refs and
// context plumbing those are tangled up in.
//
// Two ideas are worth keeping from upstream:
//
//  1. A first-character index. Typing one letter narrows 1647 emoji to the
//     few hundred whose keywords contain it, so no query ever scans the whole
//     table.
//  2. Incremental narrowing. "smi" searches the results of "sm" rather than
//     starting over, because a longer query can only ever match a subset.
//     The cache is per-picker-session and small, so it is a plain Map.

import type { EmojiRecord } from '@tinycld/core/ui/emoji-picker/emoji-data'

export interface EmojiIndex {
    /** Emoji bucketed by each alphanumeric character their keywords contain. */
    byCharacter: ReadonlyMap<string, readonly EmojiRecord[]>
    /** Keywords with punctuation and spaces stripped, joined for substring tests. */
    searchable: ReadonlyMap<EmojiRecord, string>
}

/** Letters and digits only: search ignores punctuation in keywords. */
const normalizeTerm = (raw: string) => raw.toLowerCase().replace(/[^a-z0-9]/g, '')

/**
 * Bucket every emoji under each alphanumeric character its keywords contain.
 * Built once when the lazy chunk lands.
 */
export function indexEmoji(all: readonly EmojiRecord[]): EmojiIndex {
    const byCharacter = new Map<string, EmojiRecord[]>()
    const searchable = new Map<EmojiRecord, string>()

    for (const emoji of all) {
        // Normalize the KEYWORDS too, not just the query: "thumbs up" is one
        // keyword with a space in it, so a query normalized to "thumbsup"
        // would never match it. Done once here rather than per keystroke.
        const terms = emoji.n.map(normalizeTerm)
        searchable.set(emoji, terms.join(' '))

        for (const character of new Set(terms.join(''))) {
            const bucket = byCharacter.get(character)
            if (bucket) bucket.push(emoji)
            else byCharacter.set(character, [emoji])
        }
    }

    return { byCharacter, searchable }
}

/**
 * Emoji whose keywords contain `rawQuery`, in table order.
 *
 * `cache` is optional; pass a Map that lives as long as the open picker to get
 * incremental narrowing. An empty query returns null, meaning "not searching"
 * — distinct from an empty array, which means "searched, found nothing".
 */
export function filterByKeyword(
    rawQuery: string,
    all: readonly EmojiRecord[],
    index: EmojiIndex,
    cache?: Map<string, readonly EmojiRecord[]>
): readonly EmojiRecord[] | null {
    const query = normalizeTerm(rawQuery)
    if (query === '') return null

    const cached = cache?.get(query)
    if (cached) return cached

    // Narrow from the longest previous query this one extends, else from the
    // first-character bucket, else (a query of only punctuation) everything.
    const candidates = longestPrefixMatch(query, cache) ?? index.byCharacter.get(query[0]) ?? all

    const results = candidates.filter(emoji => index.searchable.get(emoji)?.includes(query))
    cache?.set(query, results)
    return results
}

function longestPrefixMatch(
    query: string,
    cache: Map<string, readonly EmojiRecord[]> | undefined
): readonly EmojiRecord[] | null {
    if (!cache) return null
    let best: readonly EmojiRecord[] | null = null
    let bestLength = 0
    for (const [key, results] of cache) {
        if (key.length > bestLength && query.startsWith(key)) {
            best = results
            bestLength = key.length
        }
    }
    return best
}
