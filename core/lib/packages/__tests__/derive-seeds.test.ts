import { describe, expect, it } from 'vitest'
import { deriveSeeds } from '../derive-seeds'

describe('deriveSeeds', () => {
    it('orders by manifest dependencies (deps first)', () => {
        const noop = async () => {}
        const seeds = deriveSeeds([
            { manifest: { slug: 'trinkets', dependencies: ['cogs'] }, seed: noop },
            { manifest: { slug: 'cogs' }, seed: noop },
        ] as never)
        expect(seeds.map(s => s.slug)).toEqual(['cogs', 'trinkets'])
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
