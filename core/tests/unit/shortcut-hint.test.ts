import { ariaKeyShortcuts, formatHint } from '@tinycld/core/lib/shortcuts/hint'
import { describe, expect, it } from 'vitest'

describe('formatHint', () => {
    it('joins a modifier combo without "+" on Apple', () => {
        expect(formatHint('$mod+Enter', true)).toEqual({ text: '⌘↵', isPlain: false })
    })

    it('joins a modifier combo with "+" elsewhere', () => {
        expect(formatHint('$mod+Enter', false)).toEqual({ text: 'Ctrl+↵', isPlain: false })
    })

    it('marks a plain key as plain', () => {
        expect(formatHint('c', true)).toEqual({ text: 'c', isPlain: true })
    })

    it('treats Shift alone as plain: the user must release ⌘ first', () => {
        expect(formatHint('Shift+N', true)).toEqual({ text: '⇧N', isPlain: true })
    })

    it('keeps a sequence as space-separated atoms', () => {
        expect(formatHint('g i', false)).toEqual({ text: 'g i', isPlain: true })
    })

    it('treats Alt as a modifier', () => {
        expect(formatHint('Alt+s', false)).toEqual({ text: 'Alt+s', isPlain: false })
    })
})

describe('ariaKeyShortcuts', () => {
    it('resolves $mod per platform', () => {
        expect(ariaKeyShortcuts('$mod+Enter', true)).toBe('Meta+Enter')
        expect(ariaKeyShortcuts('$mod+Enter', false)).toBe('Control+Enter')
    })

    it('keeps other modifiers as they are', () => {
        expect(ariaKeyShortcuts('Shift+N', true)).toBe('Shift+N')
    })

    it('has no form for a sequence', () => {
        expect(ariaKeyShortcuts('g i', false)).toBeUndefined()
    })
})
