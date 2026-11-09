import { formatKeys } from '@tinycld/core/lib/shortcuts/keys'
import { describe, expect, it } from 'vitest'

describe('formatKeys', () => {
    it('splits sequences into separate atoms', () => {
        expect(formatKeys('t i')).toEqual([['t'], ['i']])
    })

    it('joins combo parts into one atom', () => {
        expect(formatKeys('Shift+F', false)).toEqual([['Shift', 'F']])
    })

    it('collapses Shift+<glyph> into just the glyph', () => {
        // `?` already implies Shift on a standard layout, so we don't want
        // the help overlay to render `Shift+?`.
        expect(formatKeys('Shift+?')).toEqual([['?']])
        expect(formatKeys('Shift+!')).toEqual([['!']])
    })

    it('keeps Shift when the key is a letter', () => {
        expect(formatKeys('Shift+C', false)).toEqual([['Shift', 'C']])
    })

    it('renders Enter as ↵', () => {
        expect(formatKeys('$mod+Enter')[0]).toContain('↵')
    })

    it('renders Escape as Esc', () => {
        expect(formatKeys('Escape')).toEqual([['Esc']])
    })

    it('draws the arrow keys as arrows', () => {
        expect(formatKeys('$mod+Alt+ArrowUp', true)).toEqual([['⌘', '⌥', '↑']])
        expect(formatKeys('$mod+Alt+ArrowDown', true)).toEqual([['⌘', '⌥', '↓']])
        expect(formatKeys('ArrowLeft')).toEqual([['←']])
        expect(formatKeys('ArrowRight')).toEqual([['→']])
    })

    it('draws Backspace as ⌫', () => {
        expect(formatKeys('$mod+Backspace', true)).toEqual([['⌘', '⌫']])
    })

    it('draws a letter after a modifier as a capital', () => {
        expect(formatKeys('$mod+s', true)).toEqual([['⌘', 'S']])
        expect(formatKeys('Alt+t', true)).toEqual([['⌥', 'T']])
    })

    it('keeps a digit after a modifier', () => {
        expect(formatKeys('Alt+5', true)).toEqual([['⌥', '5']])
    })

    it('keeps the case of a plain letter', () => {
        expect(formatKeys('c')).toEqual([['c']])
    })
})
