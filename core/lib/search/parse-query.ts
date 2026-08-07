import type { ParsedQuery } from './types'

// Bare boolean operators and grouping/quoting characters. Stripped rather than
// supported: every backend already strips them (core/fts sanitize.go), and
// passing incomplete expressions like `foo AND ` through to FTS5 turns a
// half-typed query into a parse error under search-as-you-type.
// Operator words must be bounded by whitespace or string edges, not word
// boundaries — \b would treat hyphens as boundaries and split hyphenated
// literals like "plan-NOT-final.docx" when the operator is stripped.
const OPERATOR_WORDS = /(^|\s)(AND|OR|NOT)(?=\s|$)/g
const OPERATOR_CHARS = /[&|!"'()]/g

/**
 * Parse palette input into scope chips and include/exclude terms.
 *
 * A word becomes a chip only when the user types `:` after it AND it names an
 * installed package — so "mail" alone stays searchable text and the email
 * titled "mail server migration" remains findable.
 */
export function parseQuery(input: string, installedSlugs: string[]): ParsedQuery {
    const slugSet = new Set(installedSlugs.map(s => s.toLowerCase()))
    const chips: string[] = []
    const include: string[] = []
    const exclude: string[] = []

    const cleaned = input.replace(OPERATOR_WORDS, '$1').replace(OPERATOR_CHARS, ' ')

    for (const rawToken of cleaned.split(/\s+/)) {
        const token = rawToken.trim()
        if (!token) continue

        if (token.endsWith(':')) {
            const candidate = token.slice(0, -1).toLowerCase()
            if (slugSet.has(candidate)) {
                if (!chips.includes(candidate)) chips.push(candidate)
                continue
            }
            // Not a package name — keep it as text, minus the trailing colon.
            const literal = token.slice(0, -1)
            if (literal) include.push(literal)
            continue
        }

        // A leading hyphen negates only when a term follows it. Because we
        // split on whitespace first, any hyphen still inside a token is
        // mid-token by construction (budget-2026) and stays literal. Strip
        // all leading hyphens (--draft becomes draft).
        if (token.startsWith('-')) {
            const term = token.replace(/^-+/, '')
            if (term) exclude.push(term)
            continue
        }

        include.push(token)
    }

    return { chips, include, exclude }
}
