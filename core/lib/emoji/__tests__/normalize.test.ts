import { CANONICAL_EMOJI, TONABLE_EMOJI } from '@tinycld/core/lib/emoji/canonical-forms'
import { isCanonicalEmoji, normalizeEmoji } from '@tinycld/core/lib/emoji/normalize'
import { applyTone, SKIN_TONES } from '@tinycld/core/lib/emoji/tones'
import { describe, expect, it } from 'vitest'
import fixture from '../__fixtures__/normalize-cases.json'

interface Case {
    raw: string
    want: string | null
    why: string
}

// This fixture is asserted from Go as well (boards/server/reaction_emoji_test.go).
// A case added here must pass there too — that is the point of it being a file
// rather than inline cases.
const cases = (fixture as { cases: Case[] }).cases

describe('normalizeEmoji — shared TS/Go fixture', () => {
    it.each(cases)('$why', ({ raw, want }) => {
        expect(normalizeEmoji(raw)).toBe(want)
    })
})

describe('canonicalization', () => {
    it('adds the variation selector NFC will not', () => {
        // The motivating case for the whole module: NFC leaves these as two
        // distinct strings, and the unique index compares bytes.
        expect('❤'.normalize('NFC')).not.toBe('❤️')
        expect(normalizeEmoji('❤')).toBe('❤️')
        expect(normalizeEmoji('❤️')).toBe('❤️')
    })

    it('is idempotent across the whole vocabulary', () => {
        for (const emoji of CANONICAL_EMOJI) {
            expect(normalizeEmoji(emoji)).toBe(emoji)
        }
    })

    it('accepts every derived tone of every tonable emoji', () => {
        // Build the toned form with applyTone, not by appending: on a ZWJ
        // sequence like 🧝‍♀️ the tone belongs on the person, before the ZWJ.
        const toUnified = (g: string) => [...g].map(c => c.codePointAt(0)!.toString(16)).join('-')
        const fromUnified = (u: string) =>
            u
                .split('-')
                .map(h => String.fromCodePoint(Number.parseInt(h, 16)))
                .join('')

        const rejected: string[] = []
        for (const base of TONABLE_EMOJI) {
            for (const tone of SKIN_TONES) {
                const toned = fromUnified(applyTone(toUnified(base), tone))
                if (normalizeEmoji(toned) !== toned) rejected.push(toned)
            }
        }
        expect(rejected).toEqual([])
    })
})

describe('validation', () => {
    it('rejects text that is not an emoji', () => {
        // The column is free text now. Without this the API could write
        // arbitrary strings that render as chips nobody can toggle off.
        expect(normalizeEmoji('not an emoji')).toBeNull()
        expect(normalizeEmoji('hello')).toBeNull()
        expect(normalizeEmoji('a')).toBeNull()
    })

    it('rejects bare digits, which carry the Unicode Emoji property', () => {
        // Why validation is a table and not a \p{Emoji} regex.
        expect(normalizeEmoji('5')).toBeNull()
        expect(normalizeEmoji('#')).toBeNull()
    })

    it('accepts keycap sequences, which a sequence regex would reject', () => {
        expect(normalizeEmoji('5️⃣')).toBe('5️⃣')
        expect(normalizeEmoji('#️⃣')).toBe('#️⃣')
    })

    it('rejects a tone on an emoji that does not take one', () => {
        expect(normalizeEmoji('🎉🏽')).toBeNull()
    })

    it('rejects country flags, which this deployment does not ship', () => {
        // Not an error so much as a documented omission: the picker cannot
        // find them, so nothing should be able to store one either.
        expect(normalizeEmoji('🇺🇦')).toBeNull()
        expect(normalizeEmoji('🇺🇸')).toBeNull()
    })

    it('keeps the non-country flags we do ship', () => {
        expect(normalizeEmoji('🏳️‍🌈')).toBe('🏳️‍🌈')
        expect(normalizeEmoji('🏴‍☠️')).toBe('🏴‍☠️')
        expect(normalizeEmoji('🏁')).toBe('🏁')
        expect(normalizeEmoji('🏴󠁧󠁢󠁳󠁣󠁴󠁿')).toBe('🏴󠁧󠁢󠁳󠁣󠁴󠁿')
    })

    it('rejects empty, blank and over-long input', () => {
        expect(normalizeEmoji('')).toBeNull()
        expect(normalizeEmoji('   ')).toBeNull()
        expect(normalizeEmoji('👍'.repeat(20))).toBeNull()
    })
})

describe('isCanonicalEmoji', () => {
    it('is the server-side check: already-canonical passes, anything else does not', () => {
        expect(isCanonicalEmoji('👍')).toBe(true)
        expect(isCanonicalEmoji('❤️')).toBe(true)
        expect(isCanonicalEmoji('❤')).toBe(false)
        expect(isCanonicalEmoji('not an emoji')).toBe(false)
    })
})
