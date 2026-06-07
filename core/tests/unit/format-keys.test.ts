import { formatKeys } from '@tinycld/core/lib/shortcuts/keys'
import { describe, expect, it } from 'vitest'

describe('formatKeys', () => {
    it('splits sequences into separate atoms', () => {
        expect(formatKeys('t i')).toEqual([['t'], ['i']])
    })

    it('joins combo parts into one atom', () => {
        expect(formatKeys('Shift+F')).toEqual([['Shift', 'F']])
    })

    it('collapses Shift+<glyph> into just the glyph', () => {
        // `?` already implies Shift on a standard layout, so we don't want
        // the help overlay to render `Shift+?`.
        expect(formatKeys('Shift+?')).toEqual([['?']])
        expect(formatKeys('Shift+!')).toEqual([['!']])
    })

    it('keeps Shift when the key is a letter', () => {
        expect(formatKeys('Shift+C')).toEqual([['Shift', 'C']])
    })

    it('renders Enter as ↵', () => {
        expect(formatKeys('$mod+Enter')[0]).toContain('↵')
    })

    it('renders Escape as Esc', () => {
        expect(formatKeys('Escape')).toEqual([['Esc']])
    })
})
