import { buildSections } from '@tinycld/core/lib/search/build-sections'
import type { SearchPackage, SearchRow } from '@tinycld/core/lib/search/types'
import { describe, expect, it } from 'vitest'

const PACKAGES: SearchPackage[] = [
    { slug: 'mail', label: 'Mail', icon: 'mail', order: 5 },
    { slug: 'drive', label: 'Drive', icon: 'hard-drive', order: 12 },
    {
        slug: 'cards',
        label: 'Cards',
        icon: 'square-kanban',
        order: 25,
    },
]

const row = (slug: string, id: string, title: string): SearchRow => ({ slug, id, title })

// buildSections arranges rows the server already ranked, so every case passes
// the server's sequence as `orderedRows` and asserts layout — never ordering.
// Ranking is covered by core/server/search's scorer tests, where rows from
// several packages can be compared on one scale.
describe('buildSections', () => {
    it('returns one flat badged section when no chips are set', () => {
        const ordered = [row('drive', 'd1', 'budget'), row('mail', 'm1', 'Q3 approval')]
        const sections = buildSections(
            { mail: [ordered[1]], drive: [ordered[0]] },
            PACKAGES,
            [],
            ordered
        )
        expect(sections).toHaveLength(1)
        expect(sections[0].title).toBeUndefined()
        expect(sections[0].showBadges).toBe(true)
        // The server's sequence is preserved verbatim — no client re-sort.
        expect(sections[0].rows.map(r => r.id)).toEqual(['d1', 'm1'])
    })

    it('returns one unbadged section when exactly one chip is set', () => {
        const ordered = [row('mail', 'm1', 'Q3'), row('mail', 'm2', 'budget')]
        const sections = buildSections({ mail: ordered }, PACKAGES, ['mail'], ordered)
        expect(sections).toHaveLength(1)
        // One package means a per-row badge would just repeat the chip.
        expect(sections[0].showBadges).toBe(false)
        expect(sections[0].rows.map(r => r.id)).toEqual(['m1', 'm2'])
    })

    it('groups by package ordered by nav.order when 2+ chips are set', () => {
        const cards = row('cards', 'c1', 'budget')
        const mail = row('mail', 'm1', 'budget')
        const sections = buildSections(
            { cards: [cards], mail: [mail] },
            PACKAGES,
            ['cards', 'mail'],
            // Server order puts cards first here; the grouped branch lays
            // sections out by nav.order regardless, which is the point.
            [cards, mail]
        )
        expect(sections.map(s => s.title)).toEqual(['Mail', 'Cards'])
        expect(sections.map(s => s.icon)).toEqual(['mail', 'square-kanban'])
    })

    it('omits a package that returned no rows', () => {
        const mail = row('mail', 'm1', 'budget')
        const sections = buildSections(
            { cards: [], mail: [mail] },
            PACKAGES,
            ['cards', 'mail'],
            [mail]
        )
        expect(sections.map(s => s.title)).toEqual(['Mail'])
    })

    it('returns no sections when nothing matched', () => {
        expect(buildSections({}, PACKAGES, [], [])).toEqual([])
    })
})
