// @vitest-environment happy-dom
//
// generateHTML/generateJSON need a DOM; core's suite defaults to node.
import { describe, expect, it } from 'vitest'
import { htmlToMarkdown, markdownToHTML } from '../html-markdown'

/**
 * The native editor exchanges HTML with its WebView but cards store markdown,
 * so these two functions sit on that boundary. They must agree with the web
 * variant's output, or the same document would be spelled differently
 * depending on which device saved it last.
 */
describe('markdown → HTML → markdown', () => {
    const cases: [string, string][] = [
        ['headings', '## Heading\n'],
        ['inline marks', 'Some **bold** and *italic* and `code`.\n'],
        ['strike', 'A ~~removed~~ phrase.\n'],
        ['bullet list', '- one\n- two\n'],
        ['ordered list', '1. one\n2. two\n'],
        ['task list', '- [ ] todo\n- [x] done\n'],
        ['blockquote', '> quoted\n'],
        ['code fence', '```go\nfmt.Println("hi")\n```\n'],
        ['link', 'See [notes](https://example.com).\n'],
        ['image', '![alt](https://example.com/i.png)\n'],
        ['table', '| a   | b   |\n| --- | --- |\n| 1   | 2   |\n'],
    ]

    for (const [label, markdown] of cases) {
        it(`round-trips ${label}`, () => {
            const back = htmlToMarkdown(markdownToHTML(markdown))
            expect(back.trim()).toBe(markdown.trim())
        })
    }

    it('is stable across a second round trip', () => {
        // Instability here would rewrite a card every time a phone opened it.
        const source = '## Notes\n\n- [x] shipped\n- [ ] pending\n\nSee `code` and **bold**.\n'
        const once = htmlToMarkdown(markdownToHTML(source))
        const twice = htmlToMarkdown(markdownToHTML(once))
        expect(twice).toBe(once)
    })

    it('preserves modifier glyphs verbatim', () => {
        const back = htmlToMarkdown(markdownToHTML('Press ⌘K then ⇧⌥P.\n'))
        expect(back).toContain('⌘K')
        expect(back).toContain('⇧⌥P')
    })

    it('repairs a code span containing a backtick', () => {
        // The raw serializer under-fences this; the repair pass must run on the
        // native path too, not just the web one.
        const back = htmlToMarkdown(markdownToHTML('A span: `` a ` b ``.\n'))
        expect(back).toContain('`` a ` b ``')
    })
})
