import { compareRows } from './score'
import type { SearchPackage, SearchRow } from './types'

export interface SearchSection {
    /** Group heading; undefined for a flat list. */
    title?: string
    /** Lucide icon name for the heading. */
    icon?: string
    /** Whether rows show a per-row package badge (flat multi-package only). */
    showBadges: boolean
    rows: SearchRow[]
}

/**
 * Arrange per-package results for rendering.
 *
 * Unscoped search is FLAT and score-ordered: it is the "I don't know where it
 * is" case, so the best answer has to be able to reach the top. Grouping would
 * pin a perfect match below every row of an earlier package. With 2+ explicit
 * chips the user has already narrowed the field, and scan-by-package is the
 * more useful affordance.
 */
export function buildSections(
    rowsBySlug: Record<string, SearchRow[]>,
    packages: SearchPackage[],
    chips: string[],
    includeTerms: string[]
): SearchSection[] {
    const orderBySlug: Record<string, number> = {}
    for (const pkg of packages) orderBySlug[pkg.slug] = pkg.order

    if (chips.length >= 2) {
        const sections: SearchSection[] = []
        for (const pkg of [...packages].sort((a, b) => a.order - b.order)) {
            if (!chips.includes(pkg.slug)) continue
            const rows = rowsBySlug[pkg.slug] ?? []
            if (rows.length === 0) continue
            sections.push({ title: pkg.label, icon: pkg.icon, showBadges: false, rows })
        }
        return sections
    }

    const all = Object.values(rowsBySlug).flat()
    if (all.length === 0) return []

    // One chip means one package, whose backend already ranked its own rows —
    // re-sorting would discard that judgement for no gain.
    if (chips.length === 1) {
        return [{ showBadges: false, rows: all }]
    }

    const sorted = [...all].sort((a, b) => compareRows(a, b, includeTerms, orderBySlug))
    return [{ showBadges: true, rows: sorted }]
}
