import { formatReactors } from '@tinycld/core/lib/reactions/names'
import { describe, expect, it } from 'vitest'

describe('formatReactors', () => {
    it('names one person', () => {
        expect(formatReactors(['Nathan'], '👍')).toBe('Nathan reacted 👍')
    })

    it('joins two with "and", not a comma', () => {
        expect(formatReactors(['Nathan', 'Sam'], '👍')).toBe('Nathan and Sam reacted 👍')
    })

    it('lists up to three in full', () => {
        expect(formatReactors(['Nathan', 'Sam', 'Ali'], '🚀')).toBe(
            'Nathan, Sam and Ali reacted 🚀'
        )
    })

    it('counts the rest beyond three', () => {
        expect(formatReactors(['Nathan', 'Sam', 'Ali', 'Kim', 'Jo'], '👍')).toBe(
            'Nathan, Sam, Ali and 2 others reacted 👍'
        )
    })

    it('says "other", singular, for exactly one more', () => {
        expect(formatReactors(['Nathan', 'Sam', 'Ali', 'Kim'], '👍')).toBe(
            'Nathan, Sam, Ali and 1 other reacted 👍'
        )
    })

    it('names the caller "You", first', () => {
        // Your own reaction is the one you are most likely checking, and
        // reading your own name back is odd.
        expect(formatReactors(['Nathan', 'Sam'], '👍', { selfIndex: 1 })).toBe(
            'You and Nathan reacted 👍'
        )
    })

    it('keeps the others in order when the caller is pulled out', () => {
        expect(formatReactors(['Nathan', 'Sam', 'Ali', 'Kim'], '👍', { selfIndex: 2 })).toBe(
            'You, Nathan, Sam and 1 other reacted 👍'
        )
    })

    it('ignores a selfIndex that is not present', () => {
        expect(formatReactors(['Nathan'], '👍', { selfIndex: -1 })).toBe('Nathan reacted 👍')
    })

    it('returns nothing for nobody', () => {
        expect(formatReactors([], '👍')).toBe('')
    })
})
