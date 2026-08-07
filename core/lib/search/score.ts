import type { SearchRow } from './types'

const TIER_EXACT_TITLE = 1000
const TIER_TITLE_PREFIX = 800
const TIER_ALL_TERMS_IN_TITLE = 600
const TIER_TITLE_SUBSTRING = 400
const TIER_SECONDARY_MATCH = 200
const TIER_NO_VISIBLE_MATCH = 100

/**
 * Fold case and punctuation so 'Budget 2026' and 'budget-2026' compare equal.
 * Punctuation becomes a space rather than being deleted, so 'budget-2026' and
 * 'budget 2026' normalize identically instead of collapsing to 'budget2026'.
 */
function normalize(value: string): string {
    return value
        .toLowerCase()
        .replace(/[^a-z0-9]+/g, ' ')
        .trim()
}

/**
 * Score how well a row matches the query, using only text the row displays.
 *
 * Deliberately NOT BM25: FTS5 ranks weight terms by frequency within their own
 * table's corpus, so scores from two packages are in different units and a
 * perfect filename match can sort below a marginal mail hit. Match quality
 * against visible text is unit-free and identical for every package.
 */
export function scoreRow(includeTerms: string[], row: SearchRow): number {
    const terms = includeTerms.map(normalize).filter(Boolean)
    if (terms.length === 0) return TIER_NO_VISIBLE_MATCH

    const title = normalize(row.title)
    const query = terms.join(' ')

    if (title === query) return TIER_EXACT_TITLE
    if (title.startsWith(query)) return TIER_TITLE_PREFIX

    const titleWords = title.split(' ')
    const everyTermPrefixesAWord = terms.every(term =>
        titleWords.some(word => word.startsWith(term))
    )
    if (everyTermPrefixesAWord) return TIER_ALL_TERMS_IN_TITLE

    if (title.includes(query)) return TIER_TITLE_SUBSTRING

    const secondary = normalize([row.subtitle, row.meta].filter(Boolean).join(' '))
    if (secondary && terms.some(term => secondary.includes(term))) return TIER_SECONDARY_MATCH

    // The backend matched something we cannot see — a mail body or drive file
    // content. Keep it, but below anything with a visible match.
    return TIER_NO_VISIBLE_MATCH
}

/**
 * Sort comparator for a flat cross-package list. Tie-breaks, in order:
 * shorter title (a tighter match), then the package's nav.order so the
 * ordering is stable while results stream in from several packages.
 */
export function compareRows(
    a: SearchRow,
    b: SearchRow,
    includeTerms: string[],
    orderBySlug: Record<string, number>
): number {
    const scoreDelta = scoreRow(includeTerms, b) - scoreRow(includeTerms, a)
    if (scoreDelta !== 0) return scoreDelta

    const lengthDelta = a.title.length - b.title.length
    if (lengthDelta !== 0) return lengthDelta

    return (orderBySlug[a.slug] ?? 0) - (orderBySlug[b.slug] ?? 0)
}
