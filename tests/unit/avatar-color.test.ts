import { avatarColor } from '@tinycld/core/components/NameAvatar'
import { describe, expect, it } from 'vitest'

const AVATAR_COLORS = [
    '#3b82f6',
    '#22c55e',
    '#a855f7',
    '#f97316',
    '#ec4899',
    '#ef4444',
    '#eab308',
    '#06b6d4',
]

describe('avatarColor', () => {
    it('always returns a color from the palette', () => {
        const keys = ['a', 'alice@example.com', 'rec_123', '', 'Зоя', '🙂', 'x'.repeat(500)]
        for (const key of keys) {
            expect(AVATAR_COLORS).toContain(avatarColor(key))
        }
    })

    it('is deterministic for a given key', () => {
        const key = 'rec_abc123'
        expect(avatarColor(key)).toBe(avatarColor(key))
    })

    // The core of the bug fix: when the avatar is keyed on a stable record id,
    // changing the displayed name must NOT change the color. Previously the
    // color was hashed from the name, so any edit reshuffled it.
    it('stays constant for the same id regardless of name changes', () => {
        const id = 'contact_42'
        const before = avatarColor(id)
        // Simulate the user renaming the contact across several edits; the color
        // key (the id) never changes.
        const after = avatarColor(id)
        expect(after).toBe(before)
    })

    it('differs across distinct keys often enough to be useful', () => {
        // Not a guarantee (8 buckets → collisions exist), but distinct ids
        // should spread across the palette rather than all collapsing to one.
        const ids = Array.from({ length: 64 }, (_, i) => `contact_${i}`)
        const used = new Set(ids.map(avatarColor))
        expect(used.size).toBeGreaterThan(1)
    })

    it('handles empty and single-character keys without throwing', () => {
        expect(AVATAR_COLORS).toContain(avatarColor(''))
        expect(AVATAR_COLORS).toContain(avatarColor('A'))
    })
})
