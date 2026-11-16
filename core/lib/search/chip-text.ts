/**
 * Pure helpers pulled out of SearchPalette.web.tsx so they can be unit tested
 * without pulling in that module's React Native / lucide-icon dependency
 * chain.
 *
 * There is deliberately no "text after chips" helper here: that used to be
 * computed by slicing on `chipsToText(chips).length`, which assumes chips are
 * always a leading prefix of the raw text. `parseQuery` recognizes `pkg:`
 * anywhere in the string, so a chip created after free text broke that
 * assumption and silently corrupted typed words. `parseQuery` now returns the
 * exact remainder itself (`ParsedQuery.remainder`) from the same pass that
 * finds the chips, so there is only one source of truth.
 */

import type { SearchRow } from './types'

/** Render a chip list back into the `slug: slug: ` prefix form. */
export function chipsToText(chips: string[]): string {
    return chips.map(c => `${c}: `).join('')
}

/**
 * Look up `row.slug`'s registered selection handler and run it.
 *
 * Returns whether a handler actually ran, so the caller (SearchPalette's
 * selectRow) can close the palette ONLY on a real selection. Rows and
 * handlers resolve through separate useQuery instances — row-building in
 * useSearchResults, handler-registration in PackageActions — so a row can
 * render before, or despite, its package's adapter module failing to load.
 * Closing unconditionally made that a silent dismiss: Enter did nothing
 * visible, indistinguishable from a working selection.
 */
export function runHandlerFor(
    row: SearchRow,
    handlers: Record<string, (row: SearchRow) => void>
): boolean {
    const handler = handlers[row.slug]
    if (!handler) return false
    handler(row)
    return true
}

type SelectHandler = (row: SearchRow) => void

/**
 * Rows arrive from the server before a package's adapter module (a lazy
 * chunk) has loaded, so a pick can land while no handler exists for its
 * package. The queue holds that one pick and runs it the moment the handler
 * registers, instead of dropping it silently: Enter on a result then always
 * opens it, just a little later on a slow load.
 */
export function createPickQueue() {
    const handlers: Record<string, SelectHandler> = {}
    let held: SearchRow | null = null
    return {
        /** Runs the row's handler now, or holds the row until its package registers one. Returns whether it ran. */
        pick(row: SearchRow): boolean {
            if (runHandlerFor(row, handlers)) {
                held = null
                return true
            }
            held = row
            return false
        },
        /** Registers a package's handler; a held row of that package runs at once. Returns whether one ran. */
        register(slug: string, handler: SelectHandler): boolean {
            handlers[slug] = handler
            if (held?.slug !== slug) return false
            const row = held
            held = null
            handler(row)
            return true
        },
    }
}
