// One emoji, one byte string — and nothing that is not an emoji.
//
// The reactions table has a unique index on (comment, user, emoji) that
// compares BYTES. Before this, `emoji` was a PocketBase select over six
// values, which made the palette the schema: a variant sequence could not
// slip past the index, and neither could arbitrary text. Opening the picker
// up to the full emoji set gave both guarantees up, and this module is what
// replaces them.
//
// Two distinct jobs, and the second is easy to forget:
//
//  1. CANONICALIZE. Unicode NFC does not insert or remove U+FE0F. "❤" (2764)
//     and "❤️" (2764 fe0f) are the same reaction to every human and every
//     font, but String.normalize('NFC') leaves them as two distinct strings —
//     so one person could react twice with the same heart and the bar would
//     render two chips. NFC alone is NOT sufficient here.
//
//  2. VALIDATE. The column is free text now. Without a membership check,
//     anything reachable through the API could write "not an emoji" into the
//     table, and it would render as a text chip that no picker can toggle off.
//
// Both are table-driven (canonical-forms.ts). A regex over Unicode emoji
// properties was tried and rejected: \p{Emoji} is true for bare digits, and
// the natural sequence pattern rejects all 15 keycap emoji, so it both over-
// and under-matches.
//
// boards/server/reaction_emoji.go implements the same rule from the same
// generated data; __fixtures__/normalize-cases.json is asserted from both
// sides so the two cannot drift.

import { CANONICAL_BY_BARE, CANONICAL_EMOJI, TONABLE_EMOJI } from './canonical-forms'
import { applyTone, isSkinTone, NEUTRAL_TONE, SKIN_TONES, stripTone } from './tones'

/** Longest emoji sequence we accept, in UTF-16 code units. Matches the column. */
const MAX_LENGTH = 32

const toUnified = (glyph: string) => [...glyph].map(c => c.codePointAt(0)!.toString(16)).join('-')

const fromUnified = (unified: string) =>
    unified
        .split('-')
        .map(hex => String.fromCodePoint(Number.parseInt(hex, 16)))
        .join('')

/**
 * The canonical byte string for `raw`, or null if it is not an emoji this
 * deployment stores.
 *
 * Rejects rather than repairs: a caller sending something unrecognizable is a
 * bug or an attack, and silently coercing it would put a value in the database
 * that no picker can display and no user can toggle off.
 */
export function normalizeEmoji(raw: string): string | null {
    if (typeof raw !== 'string') return null

    const trimmed = raw.trim()
    if (trimmed === '' || trimmed.length > MAX_LENGTH) return null

    const composed = trimmed.normalize('NFC')

    // Reactions are stored as GLYPHS; the tone helpers work in unified
    // codepoint strings. Convert at the boundary — passing a glyph to
    // stripTone silently does nothing, because splitting "👍🏽" on "-" yields
    // one element that matches no tone.
    const unified = toUnified(composed)
    const baseUnified = stripTone(unified)

    if (baseUnified !== unified) {
        // Look the base up by its tone-stripped form rather than rebuilding it
        // by string surgery. applyTone ABSORBS a variation selector, so
        // stripping a tone does not give the base back: 🕵️‍♂️ is
        // 1f575-fe0f-200d-2642-fe0f, its toned form is 1f575-1f3fe-200d-2642-fe0f,
        // and stripping the tone yields 1f575-200d-2642-fe0f — a sequence that
        // is in no table. The index below is built by applying stripTone to
        // each tonable base, so the two agree by construction.
        const base = TONABLE_BY_STRIPPED.get(baseUnified)
        if (base === undefined) return null

        const tone = unified.split('-').find(isSkinTone)
        return tone === undefined ? null : fromUnified(applyTone(base, tone))
    }

    const canonical = CANONICAL_BY_BARE[composed] ?? composed
    return CANONICAL_EMOJI.has(canonical) ? canonical : null
}

/**
 * Tone-stripped unified form -> the canonical unified base it came from.
 *
 * Built rather than generated so it cannot disagree with stripTone/applyTone;
 * 310 entries, negligible next to the table it indexes.
 */
const TONABLE_BY_STRIPPED = new Map<string, string>(
    [...TONABLE_EMOJI].map(glyph => {
        const base = toUnified(glyph)
        return [stripTone(applyTone(base, SKIN_TONES[0])), base]
    })
)

/** Whether `raw` is already exactly what we would store. */
export function isCanonicalEmoji(raw: string): boolean {
    return normalizeEmoji(raw) === raw
}

export { NEUTRAL_TONE }
