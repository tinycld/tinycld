import { describe, expect, it } from 'vitest'
import { deriveSeeds } from '../derive-seeds'

describe('deriveSeeds', () => {
    it('orders by manifest dependencies (deps first)', () => {
        const noop = async () => {}
        const seeds = deriveSeeds([
            { manifest: { slug: 'calc', dependencies: ['drive'] }, seed: noop },
            { manifest: { slug: 'drive' }, seed: noop },
        ] as never)
        expect(seeds.map(s => s.slug)).toEqual(['drive', 'calc'])
    })

    it('skips entries without a seed', () => {
        const seeds = deriveSeeds([{ manifest: { slug: 'x' } }] as never)
        expect(seeds).toHaveLength(0)
    })

    it('preserves insertion order when there are no dependencies', () => {
        const noop = async () => {}
        const seeds = deriveSeeds([
            { manifest: { slug: 'a' }, seed: noop },
            { manifest: { slug: 'b' }, seed: noop },
            { manifest: { slug: 'c' }, seed: noop },
        ] as never)
        expect(seeds.map(s => s.slug)).toEqual(['a', 'b', 'c'])
    })
})
