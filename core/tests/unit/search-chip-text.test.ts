import { chipsToText, runHandlerFor } from '@tinycld/core/lib/search/chip-text'
import type { SearchRow } from '@tinycld/core/lib/search/types'
import { describe, expect, it, vi } from 'vitest'

describe('chipsToText', () => {
    it('renders one chip as a single colon-space prefix', () => {
        expect(chipsToText(['mail'])).toBe('mail: ')
    })

    it('renders multiple chips in order, each with its own colon-space', () => {
        expect(chipsToText(['mail', 'drive'])).toBe('mail: drive: ')
    })

    it('renders no chips as an empty string', () => {
        expect(chipsToText([])).toBe('')
    })
})

// The former "text after chips" behavior (slicing the raw text by
// chipsToText(chips).length) is now covered as part of parseQuery's
// `remainder` field — see search-parse-query.test.ts. There is no standalone
// helper left to test here: computing the remainder by length assumed chips
// are always a leading prefix, which is exactly the assumption that broke
// once a chip could be created after free text.

const row: SearchRow = { slug: 'mail', id: 'm1', title: 'Q3 Budget' }

// Regression guard (I2): the palette used to close unconditionally after
// `handlers[row.slug]?.(row)`, so a row whose package's adapter module never
// registered a handler (e.g. it failed to load) made Enter a silent dismiss
// — indistinguishable from a working selection. runHandlerFor's return value
// is what SearchPalette's selectRow gates the close on.
describe('runHandlerFor', () => {
    it('runs the matching handler and reports that one ran', () => {
        const onSelect = vi.fn()
        const ran = runHandlerFor(row, { mail: onSelect })

        expect(onSelect).toHaveBeenCalledWith(row)
        expect(ran).toBe(true)
    })

    it('does nothing and reports false when no handler is registered for the slug', () => {
        const onSelect = vi.fn()
        const ran = runHandlerFor(row, { drive: onSelect })

        expect(onSelect).not.toHaveBeenCalled()
        expect(ran).toBe(false)
    })

    it('reports false against an empty handler map', () => {
        expect(runHandlerFor(row, {})).toBe(false)
    })
})
