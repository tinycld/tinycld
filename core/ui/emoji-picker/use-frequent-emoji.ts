// "Frequently used", stored per user in PocketBase rather than localStorage.
//
// Upstream keeps this in localStorage, which on this stack would mean a
// per-device list that a phone and a laptop disagree about. useUserPreference
// puts it on the user, so the emoji someone reaches for follow them.

import { useUserPreference } from '@tinycld/core/lib/use-user-preference'
import { useCallback } from 'react'

/** How many to remember. Upstream caps at 14; one row of 8 reads better here. */
const LIMIT = 8

interface FrequentEntry {
    /** The glyph as stored — a toned pick is remembered with its tone. */
    glyph: string
    count: number
}

export function useFrequentEmoji() {
    const [entries, setEntries] = useUserPreference<FrequentEntry[]>('core', 'emoji_frequent', [])

    const record = useCallback(
        (glyph: string) => {
            const existing = entries.find(entry => entry.glyph === glyph)
            const next = existing
                ? entries.map(entry =>
                      entry.glyph === glyph ? { ...entry, count: entry.count + 1 } : entry
                  )
                : [...entries, { glyph, count: 1 }]

            // Most-used first, then trim — a one-off pick ages out rather than
            // holding a slot forever.
            next.sort((a, b) => b.count - a.count)
            setEntries(next.slice(0, LIMIT))
        },
        [entries, setEntries]
    )

    return { frequent: entries.map(entry => entry.glyph), record }
}
