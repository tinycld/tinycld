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
 * Arrange already-ranked results for rendering.
 *
 * Unscoped search is FLAT and stays in the server's order: it is the "I don't
 * know where it is" case, so the best answer has to be able to reach the top.
 * Grouping would pin a perfect match below every row of an earlier package.
 * With 2+ explicit chips the user has already narrowed the field, and
 * scan-by-package is the more useful affordance.
 *
 * This function deliberately does NOT rank. Ordering is decided server-side,
 * where the aggregator can compare rows from every package against one scale;
 * re-sorting here would either duplicate that logic or silently disagree with
 * it. `orderedRows` carries the server's sequence, and the grouped branch reads
 * package order from the registry only to lay sections out.
 */
export function buildSections(
    rowsBySlug: Record<string, SearchRow[]>,
    packages: SearchPackage[],
    chips: string[],
    orderedRows: SearchRow[]
): SearchSection[] {
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

    if (orderedRows.length === 0) return []

    // One chip means one package, so a badge on every row would just repeat the
    // chip above the list.
    if (chips.length === 1) {
        return [{ showBadges: false, rows: orderedRows }]
    }

    return [{ showBadges: true, rows: orderedRows }]
}
