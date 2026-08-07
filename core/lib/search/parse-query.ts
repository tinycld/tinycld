import type { ParsedQuery } from './types'

// Grouping/quoting characters, stripped per-token rather than supported:
// every backend already strips them (core/fts sanitize.go), and passing
// incomplete expressions like `foo AND ` through to FTS5 turns a half-typed
// query into a parse error under search-as-you-type. A straight character
// class is position-independent, so applying it per-token (below) is
// equivalent to applying it to the whole string.
const OPERATOR_CHARS = /[&|!"'()]/g

// Bare boolean operator words are dropped only when a token is EXACTLY one of
// these — never a substring of a larger token — so hyphenated literals like
// "plan-NOT-final.docx" (a single whitespace-delimited token) keep NOT
// embedded and literal instead of having it stripped out.
const OPERATOR_WORD = /^(AND|OR|NOT)$/

/**
 * Parse palette input into scope chips, include/exclude terms, and the exact
 * free-text remainder the renderer should display in the input box.
 *
 * Chips are recognized ONLY from a leading, contiguous run of `pkg:` tokens
 * at the very start of the input — never from a `pkg:` token that appears
 * after other text. This mirrors the UI: chips render as their own pinned
 * row BEFORE the text field (see SearchField in SearchPalette.web.tsx), so
 * structurally a chip can only ever be a prefix. A `pkg:`-shaped token typed
 * after free text is therefore parsed as ordinary literal text, exactly like
 * a non-matching word — never silently promoted to a chip, and never
 * dropped from the query either.
 *
 * `remainder` is a substring of `input` itself — the raw text starting right
 * after the leading chip run — never re-derived by the caller. That used to
 * be the renderer's job (`textAfterChips`, computing
 * `chipsToText(chips).length` and slicing), which assumed chips were always
 * a leading prefix of the raw text. They usually are, but the RENDERED box
 * always showed `chipsToText(chips) + remainder`, a reconstruction rather
 * than the actual text — so the instant a later keystroke round-tripped that
 * reconstruction back into the store (`onChangeRemainder`), any free text
 * that preceded a chip-forming token was silently discarded. Recognizing
 * chips only as a leading run — and returning the remainder from the same
 * pass — makes `chipsToText(chips) + remainder` byte-identical to `input`
 * whenever no chip is a case/duplicate rewrite, and never lossy.
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
    // Everything after this offset in `input` (raw, unmodified) is the
    // remainder — advanced only while still inside the leading chip run.
    let remainderStart = 0
    let scanningChips = true

    // Iterate raw whitespace-delimited tokens WITH their position in `input`
    // (matchAll on \S+ rather than split, so we know exactly where each token
    // ends and can advance remainderStart to that point for chip tokens).
    for (const match of input.matchAll(/\S+/g)) {
        const rawToken = match[0]
        const tokenEnd = (match.index ?? 0) + rawToken.length
        const cleanedToken = rawToken.replace(OPERATOR_CHARS, ' ').trim()
        if (!cleanedToken) continue

        if (scanningChips) {
            const candidate = cleanedToken.endsWith(':')
                ? cleanedToken.slice(0, -1).toLowerCase()
                : null
            if (candidate && slugSet.has(candidate)) {
                if (!chips.includes(candidate)) chips.push(candidate)
                // Consume exactly one following space too, mirroring
                // chipsToText's `${slug}: ` — keeps the common case (chips
                // first, as the seeded-open and backspace-pop flows always
                // produce) byte-identical on the round trip.
                remainderStart = input[tokenEnd] === ' ' ? tokenEnd + 1 : tokenEnd
                continue
            }
            // First token that isn't a chip ends the leading run for good —
            // every later `pkg:`-shaped token is parsed as literal text below.
            scanningChips = false
        }

        if (OPERATOR_WORD.test(cleanedToken)) continue

        if (cleanedToken.endsWith(':')) {
            // Either not a package name, or a package name typed after the
            // leading chip run has already ended — either way, keep it as
            // text, minus the trailing colon.
            const literal = cleanedToken.slice(0, -1)
            if (literal) include.push(literal)
            continue
        }

        // A leading hyphen negates only when a term follows it. Because we
        // matched on \S+, any hyphen still inside a token is mid-token by
        // construction (budget-2026) and stays literal. Strip all leading
        // hyphens (--draft becomes draft).
        if (cleanedToken.startsWith('-')) {
            const term = cleanedToken.replace(/^-+/, '')
            if (term) exclude.push(term)
            continue
        }

        include.push(cleanedToken)
    }

    return { chips, include, exclude, remainder: input.slice(remainderStart) }
}
