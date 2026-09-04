// @vitest-environment happy-dom
//
// Tiptap needs a DOM to mount an editor; core's suite defaults to node.
import { Editor } from '@tiptap/core'
import { afterEach, describe, expect, it } from 'vitest'
import { buildRichEditorExtensions } from '../extensions'
import { repairMarkdown } from '../markdown-repair'

/**
 * Proves markdown is the editor's native format in the WebView.
 *
 * This exercises exactly what the page does on a `markdown.get` /
 * `markdown.set`: parse markdown in, serialize markdown out, repair on the way
 * out. Nothing pivots through HTML — which is the property the previous native
 * implementation could not offer, and the reason this task exists.
 *
 * The device path has no other mechanical coverage, so the round-trip is
 * asserted here rather than left to manual testing.
 */

let editor: Editor | null = null

afterEach(() => {
    editor?.destroy()
    editor = null
})

// Mirrors handleMarkdownMessage in webview/source/Editor.tsx.
function roundTrip(markdown: string): string {
    editor = new Editor({
        extensions: buildRichEditorExtensions(),
        content: markdown,
        contentType: 'markdown',
    })
    return repairMarkdown(editor.getMarkdown()).trim()
}

describe('WebView markdown round-trip', () => {
    it('preserves inline marks', () => {
        expect(roundTrip('**bold** and *italic* text')).toBe('**bold** and *italic* text')
    })

    it('preserves headings', () => {
        expect(roundTrip('## A heading')).toBe('## A heading')
    })

    it('preserves bullet lists', () => {
        expect(roundTrip('- one\n- two')).toBe('- one\n- two')
    })

    it('preserves task lists — checkboxes, not just the text', () => {
        // A bare StarterKit drops the checkbox state entirely.
        const out = roundTrip('- [ ] todo\n- [x] done')
        expect(out).toContain('[ ] todo')
        expect(out).toContain('[x] done')
    })

    it('preserves GFM tables', () => {
        // The serializer pads cells to align columns, which is valid GFM and
        // carries no meaning — compare cell content, not spacing, the same
        // normalization markdown-parity.test.ts applies against the Go corpus.
        const cells = (line: string) =>
            line
                .split('|')
                .map(cell => cell.trim())
                .filter(Boolean)
        const lines = roundTrip('| a | b |\n| --- | --- |\n| 1 | 2 |').split('\n')
        expect(cells(lines[0] ?? '')).toEqual(['a', 'b'])
        expect(cells(lines[2] ?? '')).toEqual(['1', '2'])
    })

    it('preserves links', () => {
        expect(roundTrip('[label](https://example.com)')).toBe('[label](https://example.com)')
    })

    it('preserves a code span containing a backtick', () => {
        // One of the two serializer corruptions markdown-repair exists to fix.
        // Unrepaired output fences this wrongly and the source is corrupted on
        // save — so this asserts the repair runs inside the WebView path.
        const out = roundTrip('Use `` a`b `` here')
        expect(out).toContain('a`b')
        expect(roundTrip(out)).toBe(out)
    })

    it('preserves blockquotes and code blocks', () => {
        expect(roundTrip('> quoted')).toBe('> quoted')
        expect(roundTrip('```\ncode line\n```')).toContain('code line')
    })

    it('preserves images, tokenless relative src included', () => {
        // The shape cards stores for a description image (see boards'
        // lib/description-image.ts): a root-relative protected-file path with
        // no token. The serializer must not mangle or absolutize it.
        const source = '![diagram](/api/files/boards_attachments/rec123/d_abc.png)'
        expect(roundTrip(source)).toBe(source)
    })

    it('inserts an image node at an explicit position', () => {
        // What commands.insertImageAt does after a drop's upload settles. The
        // position is clamped at call time — peers may have shrunk the doc.
        editor = new Editor({
            extensions: buildRichEditorExtensions(),
            content: 'para one\n\npara two',
            contentType: 'markdown',
        })
        const src = '/api/files/boards_attachments/rec123/d_abc.png'
        const max = editor.state.doc.content.size
        editor.commands.insertContentAt(Math.min(9999, max), { type: 'image', attrs: { src } })
        expect(repairMarkdown(editor.getMarkdown())).toContain(`![](${src})`)
    })

    it('is stable across a second round-trip', () => {
        // Instability here would mean every open/save rewrites the row and
        // churns the FTS index even when the user changed nothing.
        const source = '# Title\n\nSome **bold** text.\n\n- [ ] a task\n- [x] done'
        const once = roundTrip(source)
        expect(roundTrip(once)).toBe(once)
    })

    it('leaves markdown syntax literal when the format is html', () => {
        // Mail's path: the same page, contentFormat 'html'. A markdown-looking
        // string must NOT be parsed as markdown there.
        editor = new Editor({
            extensions: buildRichEditorExtensions(),
            content: '<p>**not bold**</p>',
        })
        expect(editor.getText()).toBe('**not bold**')
    })
})
