/**
 * Pure string operations on the palette's chip-prefixed text.
 *
 * Split out from SearchPalette.web.tsx (rather than left as local helpers) so
 * they can be unit tested without pulling in that module's React Native /
 * lucide-icon dependency chain.
 */

/** Render a chip list back into the `slug: slug: ` prefix form. */
export function chipsToText(chips: string[]): string {
    return chips.map(c => `${c}: `).join('')
}

/** The free-text remainder after every leading chip. */
export function textAfterChips(text: string, chips: string[]): string {
    return text.slice(chipsToText(chips).length)
}
