// The category-grouped table flattened into list rows.
//
// FlashList wants a flat array; the picker wants sections with sticky-ish
// headers and N emoji per row. This turns one into the other, and is pure so
// it can be tested without rendering anything.
//
// The column count is a PARAMETER rather than the module constant it used to
// be: a popover is sized to fit 8 cells, but a sheet is as wide as the screen
// and fits more, and a grid chunked at 8 leaves it hugging the left edge.

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
export function toRows(sections: readonly Section[], perRow: number = EMOJI_PER_ROW): EmojiRow[] {
    const columns = Math.max(1, Math.floor(perRow))
    const rows: EmojiRow[] = []
    for (const section of sections) {
        if (section.emoji.length === 0) continue
        rows.push({ kind: 'header', category: section.category, key: `h:${section.category}` })
        for (let i = 0; i < section.emoji.length; i += columns) {
            const chunk = section.emoji.slice(i, i + columns)
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
export function toSearchRows(
    results: readonly EmojiRecord[],
    perRow: number = EMOJI_PER_ROW
): EmojiRow[] {
    const columns = Math.max(1, Math.floor(perRow))
    const rows: EmojiRow[] = []
    for (let i = 0; i < results.length; i += columns) {
        const chunk = results.slice(i, i + columns)
        rows.push({ kind: 'emoji', emoji: chunk, key: `s:${chunk[0].u}` })
    }
    return rows
}
