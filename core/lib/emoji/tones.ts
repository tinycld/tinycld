// Skin tones, derived rather than stored.
//
// Upstream emoji data ships an explicit list of toned variants per emoji —
// 56KB for 329 emoji. Almost all of it is mechanical: a tone modifier goes
// directly after the base codepoint. Deriving it instead of shipping it is
// most of why the table is 97KB rather than 150KB.
//
// The two rules that are NOT obvious, both verified against all 329 upstream
// variation lists (see tones.test.ts):
//
//  1. A variation selector (fe0f) directly after the base is ABSORBED, not
//     kept: ✌️ is 270c-fe0f, but ✌🏽 is 270c-1f3fd — not 270c-1f3fd-fe0f. The
//     tone modifier already forces emoji presentation, so fe0f is redundant.
//     140 of 1645 cases; getting this wrong yields a string that renders as
//     the wrong glyph and, worse, is a DIFFERENT byte string than the one
//     every other client writes.
//
//  2. Nineteen two-person sequences (couples, holding hands, wrestling,
//     kissing) take one tone PER PERSON — 25 variants each, and the toned
//     form is a structurally different sequence, not the base plus a
//     modifier. `1f93c` (people wrestling) becomes
//     `1f468-1f3fb-200d-1faef-200d-1f468-1f3fc`. These are excluded from the
//     table's tone flag: they are offered toneless only. Reactions are the
//     consumer, a per-person tone picker is UI nobody has asked for, and
//     emoji-picker-react itself only ever applies one global tone.

export const SKIN_TONES = ['1f3fb', '1f3fc', '1f3fd', '1f3fe', '1f3ff'] as const

export type SkinTone = (typeof SKIN_TONES)[number]

/** The neutral, toneless form — what the picker shows before a tone is chosen. */
export const NEUTRAL_TONE = 'neutral'

export type ToneChoice = SkinTone | typeof NEUTRAL_TONE

const VARIATION_SELECTOR = 'fe0f'

export function isSkinTone(part: string): part is SkinTone {
    return (SKIN_TONES as readonly string[]).includes(part)
}

/**
 * The unified sequence for `unified` wearing `tone`. Returns the input
 * unchanged for the neutral tone, so callers need no special case.
 *
 * Only meaningful for emoji the table marks as tonable — applying a tone to
 * an emoji that has none produces a sequence no font will render.
 */
export function applyTone(unified: string, tone: ToneChoice): string {
    if (tone === NEUTRAL_TONE) return unified

    const [base, ...rest] = unified.split('-')
    if (rest[0] === VARIATION_SELECTOR) rest.shift()
    return [base, tone, ...rest].join('-')
}

/**
 * The toneless form of a sequence. Used to look an emoji back up in the table
 * when all we hold is a toned sequence someone reacted with.
 *
 * Note this is NOT a left inverse of applyTone: the fe0f that applyTone
 * absorbs cannot be recovered, so stripTone(applyTone('270c-fe0f', t))
 * is '270c', not '270c-fe0f'. Look up by both forms.
 */
export function stripTone(unified: string): string {
    return unified
        .split('-')
        .filter(part => !isSkinTone(part))
        .join('-')
}

/** Whether a sequence carries a tone modifier at all. */
export function hasTone(unified: string): boolean {
    return unified.split('-').some(isSkinTone)
}
