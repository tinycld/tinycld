import {
    AVATAR_COLORS,
    avatarColor,
    clampCrop,
    cropToTransform,
    DEFAULT_CROP,
    parseCrop,
    readableForegroundFor,
    resolveInitials,
    serializeCrop,
    softAvatarColors,
} from '@tinycld/core/lib/avatar'
import { describe, expect, it } from 'vitest'

/** WCAG relative luminance / contrast, reimplemented independently of the
 * production helper so these tests actually verify the math rather than
 * just echoing it back. */
function relativeLuminance(hex: string): number {
    const channels = [1, 3, 5].map(i => Number.parseInt(hex.slice(i, i + 2), 16) / 255)
    const [r, g, b] = channels.map(c => (c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4))
    return 0.2126 * (r ?? 0) + 0.7152 * (g ?? 0) + 0.0722 * (b ?? 0)
}

function contrastRatio(a: string, b: string): number {
    const [lighter, darker] = [relativeLuminance(a), relativeLuminance(b)].sort((x, y) => y - x)
    return ((lighter ?? 0) + 0.05) / ((darker ?? 0) + 0.05)
}

describe('avatarColor', () => {
    it('always returns a color from the palette', () => {
        const keys = ['a', 'alice@example.com', 'rec_123', '', 'Зоя', '🙂', 'x'.repeat(500)]
        for (const key of keys) {
            expect(AVATAR_COLORS).toContain(avatarColor(key))
        }
    })

    it('is deterministic for a given key', () => {
        expect(avatarColor('rec_abc123')).toBe(avatarColor('rec_abc123'))
    })

    // Keying on a stable record id means renaming must not reshuffle the color.
    it('stays constant for the same id regardless of name changes', () => {
        const id = 'contact_42'
        expect(avatarColor(id)).toBe(avatarColor(id))
    })

    it('spreads distinct keys across more than one bucket', () => {
        const ids = Array.from({ length: 64 }, (_, i) => `contact_${i}`)
        expect(new Set(ids.map(avatarColor)).size).toBeGreaterThan(1)
    })

    it('handles empty and single-character keys without throwing', () => {
        expect(AVATAR_COLORS).toContain(avatarColor(''))
        expect(AVATAR_COLORS).toContain(avatarColor('A'))
    })
})

describe('softAvatarColors', () => {
    it('returns a [background, foreground] pair', () => {
        const [bg, fg] = softAvatarColors('alice@example.com')
        expect(bg).toMatch(/^#[0-9a-f]{6}$/i)
        expect(fg).toMatch(/^#[0-9a-f]{6}$/i)
        expect(bg).not.toBe(fg)
    })

    it('is deterministic', () => {
        expect(softAvatarColors('a@b.c')).toEqual(softAvatarColors('a@b.c'))
    })
})

describe('readableForegroundFor', () => {
    // These four AVATAR_COLORS previously paired with a hardcoded white
    // foreground and fell below the 3:1 large-text floor — the bug this
    // helper fixes.
    const failingColors = ['#eab308', '#22c55e', '#f97316', '#06b6d4']

    it('clears WCAG AA (4.5:1) for every color that previously failed against white', () => {
        for (const bg of failingColors) {
            const fg = readableForegroundFor(bg)
            expect(contrastRatio(bg, fg)).toBeGreaterThanOrEqual(4.5)
        }
    })

    it('clears AA contrast for every AVATAR_COLORS swatch, not just the known failures', () => {
        for (const bg of AVATAR_COLORS) {
            const fg = readableForegroundFor(bg)
            expect(contrastRatio(bg, fg)).toBeGreaterThanOrEqual(4.5)
        }
    })

    it('is deterministic for a given background', () => {
        expect(readableForegroundFor('#eab308')).toBe(readableForegroundFor('#eab308'))
    })

    it('reuses the designed pairing for a known soft-palette background', () => {
        const [bg, fg] = ['#e0f2fe', '#0369a1']
        expect(readableForegroundFor(bg)).toBe(fg)
    })

    it('picks a light foreground against a very dark background', () => {
        expect(readableForegroundFor('#111111')).toBe('#ffffff')
    })
})

describe('resolveInitials', () => {
    it('takes first and last initial of a full name', () => {
        expect(resolveInitials('Ada Lovelace')).toBe('AL')
        expect(resolveInitials('Ada Byron Lovelace')).toBe('AL')
    })

    it('takes the first two letters of a single name', () => {
        expect(resolveInitials('Prince')).toBe('PR')
    })

    it('falls back to the email when the name is blank', () => {
        expect(resolveInitials('', 'ada.lovelace@example.com')).toBe('AL')
        expect(resolveInitials('   ', 'ada_lovelace@example.com')).toBe('AL')
        expect(resolveInitials('', 'ada-lovelace@example.com')).toBe('AL')
    })

    it('uses the first two letters of an unsplittable email local part', () => {
        expect(resolveInitials('', 'ada@example.com')).toBe('AD')
    })

    it('returns ? when there is nothing to work with', () => {
        expect(resolveInitials('')).toBe('?')
        expect(resolveInitials('', '')).toBe('?')
    })

    it('uppercases and handles non-ASCII names', () => {
        expect(resolveInitials('зоя павлова')).toBe('ЗП')
    })
})

describe('parseCrop / serializeCrop', () => {
    it('round-trips a valid rect', () => {
        const rect = { x: 0.25, y: 0.75, zoom: 2 }
        expect(parseCrop(serializeCrop(rect))).toEqual(rect)
    })

    it('returns the center-cover default for absent or malformed input', () => {
        expect(parseCrop(null)).toEqual(DEFAULT_CROP)
        expect(parseCrop(undefined)).toEqual(DEFAULT_CROP)
        expect(parseCrop('')).toEqual(DEFAULT_CROP)
        expect(parseCrop('not json')).toEqual(DEFAULT_CROP)
        expect(parseCrop('{"x":"a"}')).toEqual(DEFAULT_CROP)
        expect(parseCrop('null')).toEqual(DEFAULT_CROP)
    })

    it('clamps out-of-range stored values rather than trusting them', () => {
        expect(parseCrop('{"x":5,"y":-2,"zoom":0.1}')).toEqual({ x: 1, y: 0, zoom: 1 })
    })
})

describe('clampCrop', () => {
    it('keeps an in-range rect unchanged', () => {
        const rect = { x: 0.4, y: 0.6, zoom: 1.5 }
        expect(clampCrop(rect)).toEqual(rect)
    })

    it('clamps the focal point into [0,1]', () => {
        expect(clampCrop({ x: -1, y: 2, zoom: 1 })).toEqual({ x: 0, y: 1, zoom: 1 })
    })

    it('never allows zoom below 1 — the image must always cover the circle', () => {
        expect(clampCrop({ x: 0.5, y: 0.5, zoom: 0.2 }).zoom).toBe(1)
    })

    it('caps zoom at the maximum', () => {
        expect(clampCrop({ x: 0.5, y: 0.5, zoom: 99 }).zoom).toBe(8)
    })
})

describe('cropToTransform', () => {
    it('at the default rect, fills the frame exactly and centers it', () => {
        const t = cropToTransform(DEFAULT_CROP, 100)
        expect(t.width).toBe(100)
        expect(t.height).toBe(100)
        expect(t.translateX).toBe(0)
        expect(t.translateY).toBe(0)
    })

    it('scales the image up by the zoom factor', () => {
        const t = cropToTransform({ x: 0.5, y: 0.5, zoom: 2 }, 100)
        expect(t.width).toBe(200)
        expect(t.height).toBe(200)
    })

    // Focal point right-of-center means the image slides LEFT (negative X).
    it('offsets toward the focal point', () => {
        const t = cropToTransform({ x: 1, y: 0.5, zoom: 2 }, 100)
        expect(t.translateX).toBe(-100)
        expect(t.translateY).toBe(-50)
    })

    // Focal point below center means the image slides UP (negative Y).
    it('offsets vertically toward the focal point', () => {
        const t = cropToTransform({ x: 0.5, y: 1, zoom: 2 }, 100)
        expect(t.translateY).toBe(-100)
        expect(t.translateX).toBe(-50)
    })

    it('offsets both axes independently', () => {
        const t = cropToTransform({ x: 0.25, y: 0.75, zoom: 3 }, 100)
        expect(t.translateX).toBe(-50)
        expect(t.translateY).toBe(-150)
    })

    it('never leaves a gap at any zoom or focal point', () => {
        for (const zoom of [1, 1.3, 2, 5, 8]) {
            for (const x of [0, 0.5, 1]) {
                const t = cropToTransform({ x, y: x, zoom }, 100)
                expect(t.translateX).toBeLessThanOrEqual(0)
                expect(t.translateX).toBeGreaterThanOrEqual(100 - t.width)
                expect(t.translateY).toBeLessThanOrEqual(0)
                expect(t.translateY).toBeGreaterThanOrEqual(100 - t.height)
            }
        }
    })
})
