// @vitest-environment happy-dom
//
// Tiptap needs a DOM to mount an editor; core's suite defaults to node.
import { readdirSync, readFileSync } from 'node:fs'
import { join } from 'node:path'
import { Editor } from '@tiptap/core'
import { beforeAll, describe, expect, it } from 'vitest'
import { buildRichEditorExtensions } from '../extensions'
import { repairMarkdown } from '../markdown-repair'

/**
 * Cross-checks the editor's markdown against the Go serializer's golden corpus.
 *
 * The two must agree, because both write the same field: the server flush
 * serializes the shared document on save, while the client serializes what a
 * user typed. If they spelled a document differently, every save would rewrite
 * the row, churn the FTS index, and show up as a spurious edit to collaborators.
 *
 * Comparison is normalized for two cosmetic differences that carry no meaning:
 * the trailing newline (Go emits it, tiptap does not) and blank-line runs
 * (tiptap pads around tables). Content differences are real failures.
 */
const CORPUS = join(__dirname, '../../../../server/markdown/testdata/corpus')

function serialize(markdown: string): string {
    const editor = new Editor({
        element: document.createElement('div'),
        extensions: buildRichEditorExtensions(),
        content: markdown,
        contentType: 'markdown',
    })
    const out = repairMarkdown(editor.getMarkdown())
    editor.destroy()
    return out
}

// Collapse the differences that carry no meaning: tiptap pads blank lines
// around tables (including a leading one) and omits the trailing newline.
const normalize = (s: string) =>
    s
        .replace(/\n{2,}/g, '\n\n')
        .replace(/^\s+/, '')
        .replace(/\s+$/, '')

describe('markdown parity with the Go serializer', () => {
    let files: string[] = []

    beforeAll(() => {
        files = readdirSync(CORPUS)
            .filter(name => name.endsWith('.md'))
            .sort()
        // A silently empty corpus would make every assertion below vacuous.
        expect(files.length).toBeGreaterThan(0)
    })

    // 070 exercises a table cell containing an escaped pipe, which
    // @tiptap/markdown emits unescaped. That damage cannot be undone from the
    // string (see findDamagedTableRows), so the file is excluded here and
    // covered by the explicit expectation below instead. Excluding it silently
    // would hide a real limitation, so it is named.
    const KNOWN_TIPTAP_TABLE_PIPE_BUG = '070-glyphs-and-edges.md'

    it('agrees with the Go canonical spelling for every corpus file', () => {
        const differing: string[] = []
        for (const name of files) {
            if (name === KNOWN_TIPTAP_TABLE_PIPE_BUG) continue
            const source = readFileSync(join(CORPUS, name), 'utf8')
            if (normalize(serialize(source)) !== normalize(source)) {
                differing.push(name)
            }
        }
        expect(differing).toEqual([])
    })

    it('is stable on a second pass for every corpus file', () => {
        // Instability would mean a document keeps changing while nobody edits
        // it — the flush baseline would never settle.
        const unstable: string[] = []
        for (const name of files) {
            if (name === KNOWN_TIPTAP_TABLE_PIPE_BUG) continue
            const source = readFileSync(join(CORPUS, name), 'utf8')
            const once = serialize(source)
            if (serialize(once) !== once) unstable.push(name)
        }
        expect(unstable).toEqual([])
    })

    it('still loses an escaped pipe inside a table cell', () => {
        // Pins the known upstream defect. If a tiptap upgrade fixes it, this
        // test fails and the exclusions above should be removed.
        const source = readFileSync(join(CORPUS, KNOWN_TIPTAP_TABLE_PIPE_BUG), 'utf8')
        expect(source).toContain('pipe \\| inside')
        expect(serialize(source)).toContain('pipe | inside')
    })
})

describe('constructs that vanish without the right extensions', () => {
    // Each of these was silently destroyed by a bare StarterKit: task lists
    // lost their checkboxes, tables disappeared outright, images degraded to
    // their alt text. They are the reason the extension set is what it is.
    it('keeps task list checkboxes', () => {
        const out = serialize('- [ ] todo\n- [x] done\n')
        expect(out).toContain('- [ ] todo')
        expect(out).toContain('- [x] done')
    })

    it('keeps GFM tables', () => {
        const out = serialize('| a | b |\n| - | - |\n| 1 | 2 |\n')
        expect(out).toMatch(/\|\s*a\s*\|\s*b\s*\|/)
        expect(out).toMatch(/\|\s*1\s*\|\s*2\s*\|/)
    })

    it('keeps images', () => {
        expect(serialize('![alt](https://example.com/i.png)\n')).toContain(
            '![alt](https://example.com/i.png)'
        )
    })

    it('keeps a code fence language', () => {
        expect(serialize('```go\nfmt.Println("hi")\n```\n')).toContain('```go')
    })

    it('keeps strikethrough', () => {
        expect(serialize('~~gone~~\n')).toContain('~~gone~~')
    })

    it('keeps modifier glyphs verbatim', () => {
        // A user typed these; unlike an authored help topic they must never be
        // rewritten to Ctrl/Shift/Alt.
        const out = serialize('Press ⌘K then ⇧⌥P.\n')
        expect(out).toContain('⌘K')
        expect(out).toContain('⇧⌥P')
    })
})
