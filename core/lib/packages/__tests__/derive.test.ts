import { describe, expect, it } from 'vitest'
import { deriveProviders, deriveSettings, deriveSidebars } from '../derive-components'
import { deriveSeeds } from '../derive-seeds'

const SB = () => null
const PV = () => null

describe('deriveSidebars / deriveProviders', () => {
    it('maps slug → component, null when absent', () => {
        const entries = [
            { manifest: { slug: 'doodads' }, sidebar: SB },
            { manifest: { slug: 'gizmos' } },
        ]
        expect(deriveSidebars(entries)).toEqual({ doodads: SB, gizmos: null })
        const loader = { load: async () => ({ default: PV }) }
        expect(deriveProviders([{ manifest: { slug: 'x' }, provider: loader }])).toEqual({
            x: loader,
        })
    })
})

describe('deriveSettings', () => {
    it('groups panels by package, omitting packages with none', () => {
        const panels = [{ slug: 'p', label: 'P', Component: PV }]
        const entries = [
            { manifest: { name: 'Gizmos', slug: 'gizmos' }, settings: panels },
            { manifest: { name: 'Doodads', slug: 'doodads' } },
        ]
        const groups = deriveSettings(entries)
        expect(groups).toHaveLength(1)
        expect(groups[0]).toMatchObject({ packageName: 'Gizmos', pkgSlug: 'gizmos', panels })
    })
})

describe('deriveSeeds', () => {
    it('orders by dependency (deps first), skips entries without a seed', () => {
        const seed = async () => {}
        const entries = [
            { manifest: { slug: 'trinkets', dependencies: ['cogs'] }, seed },
            { manifest: { slug: 'cogs', dependencies: [] }, seed },
            { manifest: { slug: 'nodeps' } },
        ]
        const ordered = deriveSeeds(entries).map(s => s.slug)
        expect(ordered).toEqual(['cogs', 'trinkets'])
    })
})
