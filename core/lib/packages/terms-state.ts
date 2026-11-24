/**
 * The terms state of packages, from the registry rows. Pure: the hooks in
 * use-package-terms.ts feed it and every gate reads its result.
 *
 * The server owns the state (pkg_registry.terms_version and
 * terms_accepted_version, see core/server/pkgterms). The client only decides
 * what to show.
 */

export type TermsState = 'none' | 'accepted' | 'pending'

/** The registry fields the terms state is read from. */
export interface TermsRow {
    slug: string
    terms_version: string
    terms_accepted_version: string
}

/** A `declined` row of pkg_terms_decisions. */
export interface DeclinedRow {
    pkg: string
    terms_version: string
}

/** A package the owner has not accepted the current terms of. */
export interface PendingTerms {
    slug: string
    name: string
    version: string
    /** The owner declined this version. The package stays closed; no launch prompt. */
    isDeclined: boolean
}

/** A package with no registry row has no terms the owner can accept. */
export function termsStateOf(
    row: Pick<TermsRow, 'terms_version' | 'terms_accepted_version'> | undefined
): TermsState {
    if (!row || row.terms_version === '') return 'none'
    return row.terms_accepted_version === row.terms_version ? 'accepted' : 'pending'
}

/**
 * The slugs to keep closed. While the registry has not loaded, every package
 * that declares terms is closed: an open package must never show before its
 * state is known. A package that declares none is never in the set.
 */
export function blockedSlugsOf(
    declared: readonly string[],
    rows: readonly TermsRow[],
    isReady: boolean
): ReadonlySet<string> {
    if (!isReady) return new Set(declared)
    return new Set(rows.filter(r => termsStateOf(r) === 'pending').map(r => r.slug))
}

/** The pending packages, in the order of the given rows, each with its decline mark. */
export function pendingTermsOf(
    rows: readonly TermsRow[],
    declined: readonly DeclinedRow[],
    nameOf: (slug: string) => string
): PendingTerms[] {
    const declinedKeys = new Set(declined.map(d => `${d.pkg}\n${d.terms_version}`))
    return rows
        .filter(r => termsStateOf(r) === 'pending')
        .map(r => ({
            slug: r.slug,
            name: nameOf(r.slug),
            version: r.terms_version,
            isDeclined: declinedKeys.has(`${r.slug}\n${r.terms_version}`),
        }))
}

/** True when the owner must still be asked: a pending package with no decision. */
export function hasUndecidedTerms(pending: readonly PendingTerms[]): boolean {
    return pending.some(p => !p.isDeclined)
}

/** Drops the entries of closed packages from a per-package list. */
export function openEntriesOf<T>(
    entries: readonly T[],
    slugOf: (entry: T) => string,
    blocked: ReadonlySet<string>
): T[] {
    if (blocked.size === 0) return entries as T[]
    return entries.filter(e => !blocked.has(slugOf(e)))
}
