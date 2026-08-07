import { buildSections } from '@tinycld/core/lib/search/build-sections'
import type { SearchPackage, SearchRow } from '@tinycld/core/lib/search/types'
import { describe, expect, it } from 'vitest'

const PACKAGES: SearchPackage[] = [
    { slug: 'mail', label: 'Mail', icon: 'mail', order: 5, endpoint: '/api/mail/search' },
    { slug: 'drive', label: 'Drive', icon: 'hard-drive', order: 12, endpoint: '/api/drive/search' },
    {
        slug: 'cards',
        label: 'Cards',
        icon: 'square-kanban',
        order: 25,
        endpoint: '/api/cards/search',
    },
]

const row = (slug: string, id: string, title: string): SearchRow => ({ slug, id, title })

describe('buildSections', () => {
    it('returns one flat badged section when no chips are set', () => {
        const sections = buildSections(
            { mail: [row('mail', 'm1', 'Q3 approval')], drive: [row('drive', 'd1', 'budget')] },
            PACKAGES,
            [],
            ['budget']
        )
        expect(sections).toHaveLength(1)
        expect(sections[0].title).toBeUndefined()
        expect(sections[0].showBadges).toBe(true)
        // Drive's exact match outranks mail despite mail's lower nav.order.
        expect(sections[0].rows.map(r => r.id)).toEqual(['d1', 'm1'])
    })

    it('returns one unbadged section when exactly one chip is set', () => {
        const sections = buildSections(
            { mail: [row('mail', 'm1', 'Q3'), row('mail', 'm2', 'budget')] },
            PACKAGES,
            ['mail'],
            ['budget']
        )
        expect(sections).toHaveLength(1)
        expect(sections[0].showBadges).toBe(false)
        // A single package keeps its own backend rank order (m1 first despite being a worse match).
        expect(sections[0].rows.map(r => r.id)).toEqual(['m1', 'm2'])
    })

    it('groups by package ordered by nav.order when 2+ chips are set', () => {
        const sections = buildSections(
            { cards: [row('cards', 'c1', 'budget')], mail: [row('mail', 'm1', 'budget')] },
            PACKAGES,
            ['cards', 'mail'],
            ['budget']
        )
        expect(sections.map(s => s.title)).toEqual(['Mail', 'Cards'])
        expect(sections.map(s => s.icon)).toEqual(['mail', 'square-kanban'])
    })

    it('omits a package that returned no rows', () => {
        const sections = buildSections(
            { cards: [], mail: [row('mail', 'm1', 'budget')] },
            PACKAGES,
            ['cards', 'mail'],
            ['budget']
        )
        expect(sections.map(s => s.title)).toEqual(['Mail'])
    })

    it('returns no sections when nothing matched', () => {
        expect(buildSections({}, PACKAGES, [], ['budget'])).toEqual([])
    })
})
