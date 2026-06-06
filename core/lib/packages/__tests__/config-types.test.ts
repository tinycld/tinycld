import { describe, expect, it } from 'vitest'
import { definePackageEntry, type UnionToIntersection } from '../config-types'

describe('config-types inference', () => {
    it('UnionToIntersection collapses a union of objects to their intersection', () => {
        type R = UnionToIntersection<{ a: 1 } | { b: 2 }>
        const v: R = { a: 1, b: 2 }
        expect(v).toEqual({ a: 1, b: 2 })
    })

    it('definePackageEntry preserves the manifest + register value', () => {
        type S = { thing: { type: { id: string }; relations: Record<string, never> } }
        const entry = definePackageEntry<S>()({
            manifest: { name: 'X', slug: 'x', version: '0', description: '' },
            registerCollections: (nc: (n: 'thing') => { thing: 'C' }, _core: unknown) => ({
                thing: nc('thing').thing,
            }),
        })
        expect(entry.manifest.slug).toBe('x')
        expect(typeof entry.registerCollections).toBe('function')
    })

    it('definePackageEntry works for a settings-only package (no registerCollections)', () => {
        const entry = definePackageEntry<Record<string, never>>()({
            manifest: { name: 'Settings Only', slug: 'so', version: '0', description: '' },
        })
        expect(entry.manifest.slug).toBe('so')
        expect(entry.registerCollections).toBeUndefined()
    })
})
