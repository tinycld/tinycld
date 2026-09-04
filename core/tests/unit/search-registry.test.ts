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
            entry('mail', 'Mail', 'mail', 5, { endpoint: '/api/mail/search' }),
            entry('calc', 'Calc', 'table', 30),
        ])
        expect(packages.map(p => p.slug)).toEqual(['mail'])
    })

    it('sorts by nav.order', () => {
        const packages = deriveSearchPackages([
            entry('boards', 'Boards', 'square-kanban', 25, { endpoint: '/api/boards/search' }),
            entry('mail', 'Mail', 'mail', 5, { endpoint: '/api/mail/search' }),
        ])
        expect(packages.map(p => p.slug)).toEqual(['mail', 'boards'])
    })

    it('defaults the label to nav.label', () => {
        const packages = deriveSearchPackages([
            entry('mail', 'Mail', 'mail', 5, { endpoint: '/api/mail/search' }),
        ])
        expect(packages[0].label).toBe('Mail')
    })

    it('prefers an explicit search label over nav.label', () => {
        const packages = deriveSearchPackages([
            entry('mail', 'Mail', 'mail', 5, { endpoint: '/api/mail/search', label: 'Email' }),
        ])
        expect(packages[0].label).toBe('Email')
    })
})
