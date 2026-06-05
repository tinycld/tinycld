import {
    darken,
    hexToRgb,
    hexToRgba,
    readableTextColor,
    relativeLuminance,
} from '@tinycld/core/lib/color-utils'
import { describe, expect, it } from 'vitest'

describe('hexToRgb', () => {
    it('parses 6-digit hex', () => {
        expect(hexToRgb('#3b82f6')).toEqual({ r: 0x3b, g: 0x82, b: 0xf6 })
    })

    it('parses 3-digit shorthand', () => {
        expect(hexToRgb('#fff')).toEqual({ r: 255, g: 255, b: 255 })
        expect(hexToRgb('#f00')).toEqual({ r: 255, g: 0, b: 0 })
    })

    it('tolerates a missing leading hash', () => {
        expect(hexToRgb('3b82f6')).toEqual({ r: 0x3b, g: 0x82, b: 0xf6 })
    })

    it('falls back to black on malformed input rather than NaN', () => {
        expect(hexToRgb('not-a-color')).toEqual({ r: 0, g: 0, b: 0 })
        expect(hexToRgb('#12')).toEqual({ r: 0, g: 0, b: 0 })
        expect(hexToRgb('#zzzzzz')).toEqual({ r: 0, g: 0, b: 0 })
    })
})

describe('hexToRgba', () => {
    it('produces an rgba() string with the given alpha', () => {
        expect(hexToRgba('#3b82f6', 0.2)).toBe('rgba(59, 130, 246, 0.2)')
    })

    it('still works for shorthand hex', () => {
        expect(hexToRgba('#fff', 0.5)).toBe('rgba(255, 255, 255, 0.5)')
    })
})

describe('relativeLuminance', () => {
    it('is 0 for black and ~1 for white', () => {
        expect(relativeLuminance('#000000')).toBe(0)
        expect(relativeLuminance('#ffffff')).toBeCloseTo(1, 5)
    })

    it('ranks light colors above dark colors', () => {
        expect(relativeLuminance('#ffff00')).toBeGreaterThan(relativeLuminance('#3b82f6'))
        expect(relativeLuminance('#eab308')).toBeGreaterThan(relativeLuminance('#000000'))
    })
})

describe('darken', () => {
    it('returns the same color for amount 0', () => {
        expect(darken('#3b82f6', 0)).toBe('#3b82f6')
    })

    it('returns black for amount 1', () => {
        expect(darken('#3b82f6', 1)).toBe('#000000')
    })

    it('reduces luminance for partial amounts', () => {
        const original = relativeLuminance('#eab308')
        const darkened = relativeLuminance(darken('#eab308', 0.5))
        expect(darkened).toBeLessThan(original)
    })

    it('clamps out-of-range amounts', () => {
        expect(darken('#3b82f6', -1)).toBe('#3b82f6')
        expect(darken('#3b82f6', 2)).toBe('#000000')
    })

    it('always returns a 7-char hex string', () => {
        for (const a of [0, 0.25, 0.5, 0.9, 1]) {
            expect(darken('#06b6d4', a)).toMatch(/^#[0-9a-f]{6}$/)
        }
    })
})

describe('readableTextColor', () => {
    it('leaves dark colors unchanged', () => {
        for (const c of ['#3b82f6', '#ef4444', '#a855f7', '#000000']) {
            expect(readableTextColor(c)).toBe(c)
        }
    })

    // The bug from Stefan's screenshot: a yellow "Family" badge renders
    // light-on-light. The text color must be darkened to a readable shade.
    it('darkens too-light colors so the text is legible', () => {
        for (const light of ['#ffff00', '#eab308', '#ffffff', '#fef08a']) {
            const text = readableTextColor(light)
            expect(text).not.toBe(light)
            expect(relativeLuminance(text)).toBeLessThan(relativeLuminance(light))
        }
    })

    it('produces text dark enough to read on a light tint', () => {
        // After adjustment the chosen text color should be comfortably below
        // the legibility threshold.
        for (const light of ['#ffff00', '#eab308']) {
            expect(relativeLuminance(readableTextColor(light))).toBeLessThan(0.5)
        }
    })

    it('preserves hue when darkening (yellow stays yellow-ish)', () => {
        const { r, g, b } = hexToRgb(readableTextColor('#ffff00'))
        // Yellow has high R+G, low B. Darkening toward black keeps that ratio.
        expect(r).toBeGreaterThan(b)
        expect(g).toBeGreaterThan(b)
    })

    it('respects a custom threshold', () => {
        // With a threshold of 1, nothing is ever "too light".
        expect(readableTextColor('#ffff00', 1)).toBe('#ffff00')
        // With a threshold of 0, even mid colors get darkened.
        expect(readableTextColor('#3b82f6', 0)).not.toBe('#3b82f6')
    })

    it('always returns a valid hex string', () => {
        for (const c of ['#fff', '#000', '#eab308', '#06b6d4', 'garbage']) {
            expect(readableTextColor(c)).toMatch(/^#[0-9a-f]{6}$/)
        }
    })
})
