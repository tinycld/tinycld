// The category-grouped table flattened into list rows.
//
// FlashList wants a flat array; the picker wants sections with sticky-ish
// headers and 8 emoji per row. This turns one into the other, and is pure so
// it can be tested without rendering anything.

import type { EmojiRecord } from './emoji-data'
import { EMOJI_PER_ROW } from './layout'

export type EmojiRow =
    | { kind: 'header'; category: string; key: string }
    | { kind: 'emoji'; emoji: readonly EmojiRecord[]; key: string }

interface Section {
    category: string
    emoji: readonly EmojiRecord[]
}

/**
 * Sections -> rows. A section with no emoji contributes nothing, not an empty
 * header: "Frequently used" is absent until someone has used something.
 */
export function toRows(sections: readonly Section[]): EmojiRow[] {
    const rows: EmojiRow[] = []
    for (const section of sections) {
        if (section.emoji.length === 0) continue
        rows.push({ kind: 'header', category: section.category, key: `h:${section.category}` })
        for (let i = 0; i < section.emoji.length; i += EMOJI_PER_ROW) {
            const chunk = section.emoji.slice(i, i + EMOJI_PER_ROW)
            rows.push({
                kind: 'emoji',
                emoji: chunk,
                key: `${section.category}:${chunk[0].u}`,
            })
        }
    }
    return rows
}

/** Search results as rows: no headers, just the grid. */
export function toSearchRows(results: readonly EmojiRecord[]): EmojiRow[] {
    const rows: EmojiRow[] = []
    for (let i = 0; i < results.length; i += EMOJI_PER_ROW) {
        const chunk = results.slice(i, i + EMOJI_PER_ROW)
        rows.push({ kind: 'emoji', emoji: chunk, key: `s:${chunk[0].u}` })
    }
    return rows
}
