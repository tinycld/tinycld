import { describe, expect, it } from 'vitest'
import { compareStepOrder, isValidOrderKey } from '../order'

describe('isValidOrderKey', () => {
    it('accepts fractional-indexing keys', () => {
        for (const key of ['a0', 'a1', 'a0V', 'a0k', 'Zz']) expect(isValidOrderKey(key)).toBe(true)
    })
    it('rejects malformed keys', () => {
        for (const key of ['', 'a', 'a00', '10']) expect(isValidOrderKey(key)).toBe(false)
    })
})

describe('compareStepOrder', () => {
    const sort = (steps: { id: string; order: string | null }[]) =>
        [...steps].sort(compareStepOrder).map(s => s.id)

    it('sorts by plain string comparison, not locale', () => {
        expect(
            sort([
                { id: 'core:apps', order: 'a1' },
                { id: 'acme-extra:plan', order: 'Zz' },
                { id: 'acme-extra:domain', order: 'a0k' },
                { id: 'core:workspace', order: 'a0' },
                { id: 'acme-extra:web', order: 'a0V' },
            ])
        ).toEqual([
            'acme-extra:plan',
            'core:workspace',
            'acme-extra:web',
            'acme-extra:domain',
            'core:apps',
        ])
    })
    it('breaks ties by id and puts unkeyed steps last', () => {
        expect(
            sort([
                { id: 'b:x', order: null },
                { id: 'z:y', order: 'a0' },
                { id: 'a:y', order: 'a0' },
                { id: 'a:x', order: null },
            ])
        ).toEqual(['a:y', 'z:y', 'a:x', 'b:x'])
    })
})
