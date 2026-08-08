// @vitest-environment happy-dom
//
// Tiptap needs a DOM to mount an editor; core's suite defaults to node.
import { Editor } from '@tiptap/core'
import { afterEach, describe, expect, it } from 'vitest'
import { buildRichEditorExtensions } from '../extensions'
import { deriveWebViewState } from '../webview/source/state'

/**
 * The state snapshot the WebView broadcasts drives every toolbar affordance on
 * native. Exercised against a real editor rather than a stub so a schema change
 * that renames a node (and silently breaks an `isActive` probe) fails here.
 */

let editor: Editor | null = null

function mount(markdown = '', options: { characterLimit?: number } = {}) {
    editor = new Editor({
        extensions: buildRichEditorExtensions({ characterLimit: options.characterLimit }),
        content: markdown,
        contentType: 'markdown',
    })
    return editor
}

afterEach(() => {
    editor?.destroy()
    editor = null
})

describe('deriveWebViewState', () => {
    it('reports an empty document', () => {
        const state = deriveWebViewState(mount())
        expect(state.isEmpty).toBe(true)
        expect(state.characterCount).toBe(0)
        expect(state.wordCount).toBe(0)
        expect(state.isReady).toBe(true)
    })

    it('detects the active heading level', () => {
        const e = mount('## Heading')
        e.commands.setTextSelection(3)
        expect(deriveWebViewState(e).activeHeadingLevel).toBe(2)
    })

    it('reports no heading level in a paragraph', () => {
        const e = mount('plain text')
        e.commands.setTextSelection(3)
        expect(deriveWebViewState(e).activeHeadingLevel).toBeNull()
    })

    it('detects marks at the caret', () => {
        const e = mount('**bold** text')
        e.commands.setTextSelection(3)
        expect(deriveWebViewState(e).isBoldActive).toBe(true)
    })

    it('detects list context', () => {
        const e = mount('- one\n- two')
        e.commands.setTextSelection(3)
        const state = deriveWebViewState(e)
        expect(state.isBulletListActive).toBe(true)
        expect(state.isOrderedListActive).toBe(false)
    })

    it('detects task lists — the node a bare StarterKit would drop', () => {
        const e = mount('- [ ] a task')
        e.commands.setTextSelection(4)
        expect(deriveWebViewState(e).isTaskListActive).toBe(true)
    })

    it('detects table context', () => {
        const e = mount('| a | b |\n| --- | --- |\n| 1 | 2 |')
        e.commands.setTextSelection(4)
        expect(deriveWebViewState(e).isInTable).toBe(true)
    })

    it('reports the active link href', () => {
        const e = mount('[label](https://example.com)')
        e.commands.setTextSelection(3)
        const state = deriveWebViewState(e)
        expect(state.isLinkActive).toBe(true)
        expect(state.activeLink).toBe('https://example.com')
    })

    it('tracks selection emptiness', () => {
        const e = mount('some words here')
        e.commands.setTextSelection(3)
        expect(deriveWebViewState(e).selectionEmpty).toBe(true)
        e.commands.setTextSelection({ from: 2, to: 6 })
        expect(deriveWebViewState(e).selectionEmpty).toBe(false)
    })

    it('counts characters and words without a configured limit', () => {
        // CharacterCount is only registered when a limit is set, so the
        // fallback path has to hold up on its own.
        const state = deriveWebViewState(mount('one two three'))
        expect(state.wordCount).toBe(3)
        expect(state.characterCount).toBe('one two three'.length)
    })

    it('counts characters through CharacterCount when a limit is set', () => {
        const state = deriveWebViewState(mount('one two three', { characterLimit: 100 }))
        expect(state.characterCount).toBe('one two three'.length)
        expect(state.wordCount).toBe(3)
    })
})
