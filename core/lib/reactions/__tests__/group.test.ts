import { groupReactions, reactionKey } from '@tinycld/core/lib/reactions/group'
import { describe, expect, it } from 'vitest'

interface Row {
    id: string
    user: string
    emoji: string
    target: string
}

const row = (id: string, target: string, user: string, emoji: string): Row => ({
    id,
    target,
    user,
    emoji,
})
const byTarget = (r: Row) => r.target

describe('groupReactions', () => {
    it('counts per target and emoji, and finds the caller’s own row', () => {
        const groups = groupReactions(
            [
                row('r1', 't1', 'u2', '🚀'),
                row('r2', 't1', 'u1', '👍'),
                row('r3', 't1', 'u2', '👍'),
                row('r4', 't2', 'u1', '❤️'),
            ],
            'u1',
            byTarget
        )
        expect(groups.get('t1')).toEqual([
            { emoji: '👍', count: 2, ownId: 'r2', userIds: ['u1', 'u2'] },
            { emoji: '🚀', count: 1, ownId: null, userIds: ['u2'] },
        ])
        expect(groups.get('t2')).toEqual([{ emoji: '❤️', count: 1, ownId: 'r4', userIds: ['u1'] }])
        expect(groups.get('t3')).toBeUndefined()
    })

    it('keeps every reactor id, which is what the tooltip renders', () => {
        const groups = groupReactions(
            [row('r1', 't1', 'u1', '👍'), row('r2', 't1', 'u2', '👍'), row('r3', 't1', 'u3', '👍')],
            'u2',
            byTarget
        )
        expect(groups.get('t1')?.[0].userIds).toEqual(['u1', 'u2', 'u3'])
    })

    it('orders by count, not arrival', () => {
        const groups = groupReactions(
            [
                row('r1', 't1', 'u1', '🐌'),
                row('r2', 't1', 'u2', '🚀'),
                row('r3', 't1', 'u3', '🚀'),
                row('r4', 't1', 'u4', '🎉'),
                row('r5', 't1', 'u5', '🎉'),
                row('r6', 't1', 'u6', '🎉'),
            ],
            '',
            byTarget
        )
        expect(groups.get('t1')?.map(g => g.emoji)).toEqual(['🎉', '🚀', '🐌'])
    })

    it('breaks ties on first appearance, so equal counts do not shuffle', () => {
        const rows = [
            row('r1', 't1', 'u1', '👀'),
            row('r2', 't1', 'u2', '👍'),
            row('r3', 't1', 'u3', '🎉'),
        ]
        expect(
            groupReactions(rows, '', byTarget)
                .get('t1')
                ?.map(g => g.emoji)
        ).toEqual(['👀', '👍', '🎉'])
    })

    it('never claims a row for an anonymous reader', () => {
        // The public-board case: no session, so no group is "yours".
        const groups = groupReactions([row('r1', 't1', 'u1', '👍')], '', byTarget)
        expect(groups.get('t1')?.[0].ownId).toBeNull()
    })

    it('treats a skin tone as its own reaction', () => {
        const groups = groupReactions(
            [row('r1', 't1', 'u1', '👍'), row('r2', 't1', 'u2', '👍🏽')],
            '',
            byTarget
        )
        expect(groups.get('t1')).toHaveLength(2)
    })

    it('groups by whatever key it is given', () => {
        // The same rows keyed two ways: one fold serves comment and card
        // reactions rather than a near-identical copy per table.
        const rows = [row('r1', 't1', 'u1', '👍'), row('r2', 't2', 'u1', '👍')]
        expect(groupReactions(rows, 'u1', byTarget).size).toBe(2)
        expect(groupReactions(rows, 'u1', () => 'all').size).toBe(1)
    })

    it('returns nothing for no rows', () => {
        expect(groupReactions([], 'u1', byTarget).size).toBe(0)
    })
})

describe('reactionKey', () => {
    it('is the unified codepoints, so a selector never carries a raw glyph', () => {
        expect(reactionKey('👍')).toBe('1f44d')
        expect(reactionKey('❤️')).toBe('2764-fe0f')
        expect(reactionKey('👍🏽')).toBe('1f44d-1f3fd')
    })

    it('distinguishes a toned emoji from its base', () => {
        expect(reactionKey('👍')).not.toBe(reactionKey('👍🏽'))
    })
})
