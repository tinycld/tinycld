// @vitest-environment happy-dom
//
// Tiptap needs a DOM to mount an editor; core's suite defaults to node.
import { Editor } from '@tiptap/core'
import { afterEach, describe, expect, it } from 'vitest'
import { buildRichEditorExtensions } from '../extensions'
import { deriveWebViewState } from '../webview/source/state'

/**
 * `activeHeadingLevel` is derived twice — once inside the WebView page for
 * native, once in the web hook's toolbarState literal — because the two
 * platforms reach the editor by different routes. Nothing but this test stops
 * them drifting apart, and drift here is invisible: a heading button that
 * lights up on a phone and stays dark in a browser looks like a styling bug,
 * not a state bug.
 *
 * The web hook cannot be rendered here without dragging in theme providers and
 * RN-Web, so it is the DERIVATION that is pinned: the expression below is a
 * literal copy of the one in use-rich-editor.web.tsx, checked against the
 * WebView's own answer for the same document.
 */

const HEADING_LEVELS = [1, 2, 3, 4, 5, 6]

/** Mirrors the `activeHeadingLevel` line in use-rich-editor.web.tsx. */
function webActiveHeadingLevel(editor: Editor): number | null {
    return HEADING_LEVELS.find(level => editor.isActive('heading', { level })) ?? null
}

let editor: Editor | null = null

function mount(markdown: string) {
    editor = new Editor({
        extensions: buildRichEditorExtensions(),
        content: markdown,
        contentType: 'markdown',
    })
    return editor
}

afterEach(() => {
    editor?.destroy()
    editor = null
})

describe('activeHeadingLevel parity between web and the WebView', () => {
    it.each([
        ['# One', 1],
        ['## Two', 2],
        ['### Three', 3],
        ['plain paragraph', null],
    ])('agrees on %s', (markdown, expected) => {
        const e = mount(markdown)
        e.commands.setTextSelection(3)

        expect(webActiveHeadingLevel(e)).toBe(expected)
        expect(deriveWebViewState(e).activeHeadingLevel).toBe(expected)
    })

    it('follows the caret between a heading and the paragraph under it', () => {
        const e = mount('## Scope\n\nplain body text')

        e.commands.setTextSelection(3)
        expect(webActiveHeadingLevel(e)).toBe(2)
        expect(deriveWebViewState(e).activeHeadingLevel).toBe(2)

        // Past the heading node and into the paragraph.
        e.commands.setTextSelection(e.state.doc.content.size - 2)
        expect(webActiveHeadingLevel(e)).toBeNull()
        expect(deriveWebViewState(e).activeHeadingLevel).toBeNull()
    })
})
