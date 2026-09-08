// The picker's state: query, tone, and the rows the grid renders.
//
// Kept out of the component so the row assembly (which has the fiddly bits —
// search vs categories, the frequent section, tone application) is testable
// without rendering a FlashList.

import { parseNativeEmoji } from '@tinycld/core/lib/emoji/parse'
import { filterByKeyword } from '@tinycld/core/lib/emoji/search'
import { NEUTRAL_TONE, stripTone, type ToneChoice } from '@tinycld/core/lib/emoji/tones'
import { useUserPreference } from '@tinycld/core/lib/use-user-preference'
import { useMemo, useRef, useState } from 'react'
import { CATEGORY_ORDER, FREQUENT_CATEGORY } from './categories'
import type { EmojiRecord } from './emoji-data'
import { toRows, toSearchRows } from './rows'
import type { EmojiTable } from './use-emoji-data'
import { useFrequentEmoji } from './use-frequent-emoji'

export function useEmojiPicker(table: EmojiTable | null) {
    const [query, setQuery] = useState('')
    const [tone, setTone] = useUserPreference<ToneChoice>('core', 'emoji_skin_tone', NEUTRAL_TONE)
    const { frequent, record } = useFrequentEmoji()

    // Lives as long as the open picker: incremental narrowing reuses the
    // previous query's results, so typing does not rescan the table.
    const searchCache = useRef(new Map<string, readonly EmojiRecord[]>())

    const results = useMemo(() => {
        if (!table) return null
        return filterByKeyword(query, table.all, table.index, searchCache.current)
    }, [query, table])

    const rows = useMemo(() => {
        if (!table) return []
        if (results) return toSearchRows(results)

        const byCategory = new Map(table.categories.map(section => [section.c, section.e]))
        // A remembered pick may carry a tone; the table is keyed by the
        // toneless glyph, so strip before looking up. The grid re-applies the
        // current tone when it renders.
        const frequentRecords = frequent
            .map(glyph => table.byGlyph.get(glyph) ?? table.byGlyph.get(stripToneGlyph(glyph)))
            .filter((emoji): emoji is EmojiRecord => emoji !== undefined)

        return toRows(
            CATEGORY_ORDER.map(category => ({
                category,
                emoji:
                    category === FREQUENT_CATEGORY
                        ? frequentRecords
                        : (byCategory.get(category) ?? []),
            }))
        )
    }, [table, results, frequent])

    return {
        query,
        setQuery,
        tone,
        setTone,
        rows,
        isSearching: results !== null,
        recordUse: record,
    }
}

/** Glyph -> toneless glyph, via the unified form the tone helpers work in. */
function stripToneGlyph(glyph: string): string {
    const unified = [...glyph].map(c => (c.codePointAt(0) ?? 0).toString(16)).join('-')
    return parseNativeEmoji(stripTone(unified))
}
