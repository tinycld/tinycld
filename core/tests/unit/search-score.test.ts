import { compareRows, scoreRow } from '@tinycld/core/lib/search/score'
import type { SearchRow } from '@tinycld/core/lib/search/types'
import { describe, expect, it } from 'vitest'

const row = (over: Partial<SearchRow>): SearchRow => ({
    slug: 'mail',
    id: '1',
    title: 'untitled',
    ...over,
})

describe('scoreRow — tiers', () => {
    it('ranks an exact title match highest', () => {
        expect(scoreRow(['budget'], row({ title: 'budget' }))).toBeGreaterThan(
            scoreRow(['budget'], row({ title: 'budget review' }))
        )
    })

    it('ranks a title prefix above a word-prefix match', () => {
        expect(scoreRow(['budget'], row({ title: 'budget review' }))).toBeGreaterThan(
            scoreRow(['budget'], row({ title: 'Q3 budgeting notes' }))
        )
    })

    it('ranks a title match above a subtitle-only match', () => {
        expect(scoreRow(['grace'], row({ title: 'grace period' }))).toBeGreaterThan(
            scoreRow(['grace'], row({ title: 'Q3 approval', subtitle: 'Grace Hopper' }))
        )
    })

    it('ranks a subtitle match above a body-only hit with no visible match', () => {
        expect(
            scoreRow(['budget'], row({ title: 'Q3 approval', subtitle: 'budget team' }))
        ).toBeGreaterThan(scoreRow(['budget'], row({ title: 'Q3 approval' })))
    })

    it('is case- and punctuation-insensitive on an exact match', () => {
        expect(scoreRow(['budget-2026'], row({ title: 'Budget 2026' }))).toBe(
            scoreRow(['budget-2026'], row({ title: 'budget-2026' }))
        )
    })

    it('normalizes punctuation by replacement, not deletion', () => {
        // With separate terms ['budget', '2026'], a replacement normalizer joins them
        // with spaces ('budget 2026') which matches 'budget-2026'. Deletion would create
        // 'budget2026' and then fail to split back to separate terms, scoring lower.
        expect(scoreRow(['budget', '2026'], row({ title: 'budget-2026' }))).toBe(
            scoreRow(['budget-2026'], row({ title: 'budget-2026' }))
        )
    })

    it('requires every term to match for the all-terms tier', () => {
        const both = scoreRow(['q3', 'budget'], row({ title: 'Q3 budget plan' }))
        const one = scoreRow(['q3', 'budget'], row({ title: 'Q3 plan' }))
        expect(both).toBeGreaterThan(one)
    })

    it('handles degenerate inputs (empty or punctuation-only queries)', () => {
        const empty = scoreRow([], row({ title: 'x' }))
        const punctuationOnly = scoreRow(['---'], row({ title: 'x' }))
        // Both should land on the lowest tier (no visible match).
        expect(empty).toBe(punctuationOnly)
    })
})

describe('compareRows — cross-package ordering', () => {
    const order = { mail: 5, drive: 12, cards: 25 }

    // The case that motivates scoring at all: without it, nav.order alone
    // would put Mail's weak hit above Drive's exact filename match.
    it('puts a high-tier hit from a later package above a low-tier earlier one', () => {
        const driveExact = row({ slug: 'drive', id: 'd1', title: 'budget-2026' })
        const mailWeak = row({
            slug: 'mail',
            id: 'm1',
            title: 'Q3 approval',
            subtitle: 'budget team',
        })
        const sorted = [mailWeak, driveExact].sort((a, b) =>
            compareRows(a, b, ['budget-2026'], order)
        )
        expect(sorted[0].id).toBe('d1')
    })

    it('prefers the shorter title within a tier', () => {
        const short = row({ slug: 'mail', id: 'a', title: 'budget review' })
        const long = row({
            slug: 'drive',
            id: 'b',
            title: 'budget review for the third quarter of the year',
        })
        const sorted = [long, short].sort((a, b) => compareRows(a, b, ['budget'], order))
        expect(sorted[0].id).toBe('a')
    })

    it('falls back to nav.order when tier and title length tie', () => {
        const cards = row({ slug: 'cards', id: 'c', title: 'budget' })
        const mail = row({ slug: 'mail', id: 'm', title: 'budget' })
        const sorted = [cards, mail].sort((a, b) => compareRows(a, b, ['budget'], order))
        expect(sorted[0].id).toBe('m')
    })

    it('is deterministic regardless of input order', () => {
        const rows = [
            row({ slug: 'cards', id: 'c', title: 'budget plan' }),
            row({ slug: 'drive', id: 'd', title: 'budget' }),
            row({ slug: 'mail', id: 'm', title: 'Q3', subtitle: 'budget' }),
        ]
        const forward = [...rows].sort((a, b) => compareRows(a, b, ['budget'], order))
        const backward = [...rows].reverse().sort((a, b) => compareRows(a, b, ['budget'], order))
        expect(forward.map(r => r.id)).toEqual(backward.map(r => r.id))
    })
})
