import { escapeMarkdown } from '@tinycld/core/lib/markdown-escape'
import { describe, expect, it } from 'vitest'

/**
 * A display name is user-controlled and gets spliced into markdown source when
 * a mention renders. Without escaping, the name IS markup in someone else's
 * comment.
 */
describe('escapeMarkdown', () => {
    it('neutralizes a link construct', () => {
        const name = '[click](javascript:alert(1))'
        const escaped = escapeMarkdown(name)

        expect(escaped).not.toContain('](')
        // The text still reads as what the user typed, just inert.
        expect(escaped.replace(/\\/g, '')).toBe(name)
    })

    it('neutralizes an image construct', () => {
        // Worse than a link: an image needs no tap. The reader's client fetches
        // it on render, so an unescaped name is a tracking pixel in every
        // comment that mentions its owner.
        const escaped = escapeMarkdown('![x](https://evil.test/track.png)')

        expect(escaped).not.toContain('![')
        expect(escaped).not.toContain('](')
    })

    it('escapes a backslash so an escape cannot be un-escaped', () => {
        // `\` first in the character class, or `\[` would become `\\[` — a
        // literal backslash followed by a live bracket.
        expect(escapeMarkdown('a\\[b](c)')).toBe('a\\\\\\[b\\]\\(c\\)')
    })

    it('leaves ordinary names untouched', () => {
        expect(escapeMarkdown('Ada Lovelace')).toBe('Ada Lovelace')
        expect(escapeMarkdown('李雷')).toBe('李雷')
        expect(escapeMarkdown("O'Brien")).toBe("O'Brien")
    })

    it('escapes emphasis and code punctuation', () => {
        expect(escapeMarkdown('*bold*')).toBe('\\*bold\\*')
        expect(escapeMarkdown('_em_')).toBe('\\_em\\_')
        expect(escapeMarkdown('`code`')).toBe('\\`code\\`')
    })

    it('escapes constructs that only bite at the start of a line', () => {
        // A name is spliced mid-sentence today, but "@- item" or "@# Heading"
        // becomes block markup the moment a mention opens a line.
        expect(escapeMarkdown('- item')).toBe('\\- item')
        expect(escapeMarkdown('# Heading')).toBe('\\# Heading')
        expect(escapeMarkdown('> quote')).toBe('\\> quote')
    })

    it('escapes a pipe so a name cannot break a table row', () => {
        expect(escapeMarkdown('a|b')).toBe('a\\|b')
    })

    it('is a no-op on the empty string', () => {
        expect(escapeMarkdown('')).toBe('')
    })
})
