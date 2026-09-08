import {
    applyTone,
    hasTone,
    NEUTRAL_TONE,
    SKIN_TONES,
    stripTone,
} from '@tinycld/core/lib/emoji/tones'
import { describe, expect, it } from 'vitest'
import upstream from '../__fixtures__/upstream-tones.json'

// The fixture is every skin-tone variation emoji-picker-react ships, captured
// verbatim. Deriving tones instead of storing them only pays off if the
// derivation is exactly right, so this asserts against all 310 rather than a
// hand-picked sample: a wrong sequence renders as the wrong glyph AND is a
// different byte string than every other client writes, which silently splits
// one reaction into two chips.
const tonable = upstream.tonable as Record<string, (string | null)[]>

describe('applyTone', () => {
    it('reproduces every upstream toned variant', () => {
        const mismatches: string[] = []
        for (const [base, variants] of Object.entries(tonable)) {
            SKIN_TONES.forEach((tone, i) => {
                const expected = variants[i]
                if (!expected) return
                const actual = applyTone(base, tone)
                if (actual !== expected) {
                    mismatches.push(`${base} + ${tone} -> ${actual}, want ${expected}`)
                }
            })
        }
        expect(mismatches).toEqual([])
    })

    it('covers the full set, not a token subset', () => {
        expect(Object.keys(tonable).length).toBe(310)
    })

    it('absorbs a variation selector rather than keeping it', () => {
        // ✌️ is 270c-fe0f but ✌🏽 is 270c-1f3fd — the tone modifier already
        // forces emoji presentation. Appending would give 270c-1f3fd-fe0f,
        // which is a different byte string than every other client writes.
        expect(applyTone('270c-fe0f', '1f3fd')).toBe('270c-1f3fd')
        expect(applyTone('261d-fe0f', '1f3fb')).toBe('261d-1f3fb')
    })

    it('splices after the base, not at the end, for ZWJ sequences', () => {
        // 🧑‍🚀 astronaut: the tone belongs to the person, before the ZWJ.
        expect(applyTone('1f9d1-200d-1f680', '1f3fe')).toBe('1f9d1-1f3fe-200d-1f680')
    })

    it('returns the input unchanged for the neutral tone', () => {
        expect(applyTone('1f44d', NEUTRAL_TONE)).toBe('1f44d')
        expect(applyTone('270c-fe0f', NEUTRAL_TONE)).toBe('270c-fe0f')
    })
})

describe('stripTone', () => {
    it('recovers the base for every derived variant', () => {
        // Not a strict inverse: the fe0f applyTone absorbs cannot come back,
        // so compare modulo the variation selector.
        const withoutVs = (u: string) =>
            u
                .split('-')
                .filter(p => p !== 'fe0f')
                .join('-')
        for (const base of Object.keys(tonable)) {
            for (const tone of SKIN_TONES) {
                expect(withoutVs(stripTone(applyTone(base, tone)))).toBe(withoutVs(base))
            }
        }
    })

    it('leaves a toneless sequence alone', () => {
        expect(stripTone('1f44d')).toBe('1f44d')
        expect(stripTone('1f9d1-200d-1f680')).toBe('1f9d1-200d-1f680')
    })
})

describe('hasTone', () => {
    it('distinguishes toned from toneless', () => {
        expect(hasTone('1f44d')).toBe(false)
        expect(hasTone('1f44d-1f3fd')).toBe(true)
        expect(hasTone('1f9d1-1f3fe-200d-1f680')).toBe(true)
    })
})

describe('multi-tone sequences', () => {
    it('are excluded from tone support, and documented as such', () => {
        // Nineteen two-person sequences (couples, wrestling, holding hands)
        // take one tone PER PERSON — 25 variants each — and the toned form is
        // a structurally different sequence, not the base plus a modifier.
        // They are offered toneless only; this asserts the list has not
        // silently grown on an upstream refresh.
        expect(upstream.multiTone).toHaveLength(19)
        expect(upstream.multiTone).toContain('1f91d') // 🤝 agreement
        expect(upstream.multiTone).toContain('1f46b') // 👫 woman and man holding hands
    })

    it('would not reconstruct by rule, which is why they are excluded', () => {
        // 🤝 toned is 1faf1-<tone>-200d-1faf2-<tone>, nothing like the base.
        expect(applyTone('1f91d', '1f3fb')).not.toBe('1faf1-1f3fb-200d-1faf2-1f3fb')
    })
})
