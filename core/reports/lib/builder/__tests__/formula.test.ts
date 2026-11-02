import { describe, expect, it } from 'vitest'
import { hasScale, onlyKeys } from '../formula'

describe('the shape checks report formulas share', () => {
    it('reads a node as one shape only when it sets no other key', () => {
        expect(onlyKeys({ col: 'amount' }, ['col'])).toBe(true)
        expect(onlyKeys({ col: 'amount', scale: undefined }, ['col'])).toBe(true)
        expect(onlyKeys({ col: 'amount', scale: 2 }, ['col'])).toBe(false)
    })

    it('accepts a scale of 0 to 6 places, as the engine does', () => {
        expect(hasScale({ op: 'div', scale: 0 })).toBe(true)
        expect(hasScale({ op: 'div', scale: 6 })).toBe(true)
        expect(hasScale({ op: 'div', scale: 7 })).toBe(false)
        expect(hasScale({ op: 'div' })).toBe(false)
    })
})
