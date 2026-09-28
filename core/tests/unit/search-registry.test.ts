import { deriveSearchPackages } from '@tinycld/core/lib/search/registry'
import { describe, expect, it } from 'vitest'

const entry = (
    slug: string,
    label: string,
    icon: string,
    order: number,
    search?: { endpoint: string; label?: string }
) => ({
    manifest: { slug, name: label, nav: { label, icon, order } },
    search: search
        ? {
              ...search,
              load: async () => ({
                  toRow: () => null,
                  useSearchActions: () => ({ onSelect: () => {} }),
              }),
          }
        : undefined,
})

describe('deriveSearchPackages', () => {
    it('includes only packages declaring search', () => {
        const packages = deriveSearchPackages([
            entry('gizmos', 'Gizmos', 'gizmos', 5, { endpoint: '/api/gizmos/search' }),
            entry('trinkets', 'Trinkets', 'table', 30),
        ])
        expect(packages.map(p => p.slug)).toEqual(['gizmos'])
    })

    it('sorts by nav.order', () => {
        const packages = deriveSearchPackages([
            entry('gadgets', 'Gadgets', 'square-kanban', 25, { endpoint: '/api/gadgets/search' }),
            entry('gizmos', 'Gizmos', 'gizmos', 5, { endpoint: '/api/gizmos/search' }),
        ])
        expect(packages.map(p => p.slug)).toEqual(['gizmos', 'gadgets'])
    })

    it('defaults the label to nav.label', () => {
        const packages = deriveSearchPackages([
            entry('gizmos', 'Gizmos', 'gizmos', 5, { endpoint: '/api/gizmos/search' }),
        ])
        expect(packages[0].label).toBe('Gizmos')
    })

    it('prefers an explicit search label over nav.label', () => {
        const packages = deriveSearchPackages([
            entry('gizmos', 'Gizmos', 'gizmos', 5, {
                endpoint: '/api/gizmos/search',
                label: 'Email',
            }),
        ])
        expect(packages[0].label).toBe('Email')
    })
})
