// The only module allowed to reference emoji-data.
//
// Metro does not tree-shake (see metro.config.cjs, where Lucide is rewritten
// to per-icon deep imports because the barrel costs ~3.3MB). A static import
// of the emoji table would put ~99KB in the base bundle for every user on
// every platform, including deployments with no package that has a picker.
// So the table is reached ONLY through import(), and bundle-sentinel.test.tsx
// asserts that no other module imports it.
//
// The chunk is fetched at most once per session: the promise itself is cached,
// so concurrent openings of the picker share one request rather than racing.

import { parseNativeEmoji } from '@tinycld/core/lib/emoji/parse'
import { type EmojiIndex, indexEmoji } from '@tinycld/core/lib/emoji/search'
import { useEffect, useState } from 'react'
import type { EmojiCategory, EmojiRecord } from './emoji-data'

export interface EmojiTable {
    categories: readonly EmojiCategory[]
    /** Every emoji, flattened, for search. */
    all: readonly EmojiRecord[]
    /** First-character index, so a query narrows without scanning all 1647. */
    index: EmojiIndex
    /**
     * Glyph -> record, for looking an emoji back up from a stored value.
     * Keyed by the toneless glyph: "frequently used" remembers 👍🏽 as picked,
     * but the record it maps to is 👍's.
     */
    byGlyph: ReadonlyMap<string, EmojiRecord>
}

let pending: Promise<EmojiTable> | null = null

function loadTable(): Promise<EmojiTable> {
    pending ??= import('./emoji-data').then(({ EMOJI_DATA }) => {
        const all = EMOJI_DATA.flatMap(category => category.e)
        const byGlyph = new Map(all.map(emoji => [parseNativeEmoji(emoji.u), emoji]))
        return { categories: EMOJI_DATA, all, index: indexEmoji(all), byGlyph }
    })
    return pending
}

/**
 * The emoji table, or null while the chunk is in flight.
 *
 * `enabled` gates the fetch so a mounted-but-closed picker costs nothing —
 * the table loads when someone actually opens one.
 */
export function useEmojiData(enabled: boolean): { table: EmojiTable | null; isLoading: boolean } {
    const [table, setTable] = useState<EmojiTable | null>(null)

    useEffect(() => {
        if (!enabled || table) return
        let cancelled = false
        loadTable().then(loaded => {
            if (!cancelled) setTable(loaded)
        })
        return () => {
            cancelled = true
        }
    }, [enabled, table])

    return { table, isLoading: enabled && !table }
}

/** Warm the chunk ahead of an open — e.g. on hover of a picker trigger. */
export function preloadEmojiData(): void {
    void loadTable()
}
