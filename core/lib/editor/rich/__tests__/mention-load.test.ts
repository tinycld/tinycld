// @vitest-environment happy-dom
//
// Tiptap needs a DOM to mount an editor; core's suite defaults to node.
import { Editor } from '@tiptap/core'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { buildRichEditorExtensions } from '../extensions'
import { resetMentionLabels, setMentionLabels } from '../mention-node'

// Mentions arrive as TOKEN TEXT in stored content, and must become nodes
// however that content reaches the editor — opening an existing comment, a Yjs
// update from a collaborator, a paste. The input rule only fires while typing,
// so it does not cover any of those: an existing comment opened showing the raw
// `[[@id|Name]]`.
//
// The round trip is the assertion that matters. An earlier attempt rewrote the
// markdown into an HTML span before parsing, which the native page did not
// parse — and the next blur SAVED the literal markup, turning a comment into
// `Hi @admin@admin&lt;/span&gt;`. Real data loss, caught on a device rather than
// here, which is why these tests exist.

// Every editor made here is destroyed after its test. ProseMirror's DOMObserver
// schedules a flush on a timer, and that flush reads `document` — so an editor
// left alive past teardown throws "document is not defined" from a stray timer,
// surfacing as an uncaught exception that fails the run even though every
// assertion passed.
const openEditors: Editor[] = []

function makeEditor() {
    const editor = new Editor({
        extensions: buildRichEditorExtensions({
            triggers: [
                {
                    id: 'm',
                    char: '@',
                    allItems: [{ id: 'u1', label: 'Ada' }],
                    insertTemplate: '[[@{id}]] ',
                    insertsMentionNode: true,
                    onStateChange: () => {},
                },
            ],
        }),
    })
    openEditors.push(editor)
    return editor
}

describe('loading content that contains mentions', () => {
    beforeEach(() => {
        resetMentionLabels()
        setMentionLabels('m', [{ id: 'u1', label: 'Ada' }])
    })

    afterEach(() => {
        for (const editor of openEditors.splice(0)) editor.destroy()
    })

    it('turns a stored token into a mention node', () => {
        const editor = makeEditor()
        editor.commands.setContent('Hi [[@u1|Ada]]', { contentType: 'markdown' as never })
        expect(JSON.stringify(editor.getJSON())).toContain('tinycldMention')
    })

    // The whole point: what was loaded must serialize back byte-identically, or
    // merely opening a comment and closing it corrupts the stored text.
    it('round-trips back to the same markdown', () => {
        const editor = makeEditor()
        editor.commands.setContent('Hi [[@u1|Ada]]', { contentType: 'markdown' as never })
        expect(editor.getMarkdown().trim()).toBe('Hi [[@u1|Ada]]')
    })

    it('never leaves markup in the saved text', () => {
        const editor = makeEditor()
        editor.commands.setContent('Hi [[@u1|Ada]]', { contentType: 'markdown' as never })
        const out = editor.getMarkdown()
        expect(out).not.toContain('span')
        expect(out).not.toContain('&lt;')
    })

    it('handles the legacy token that carries no name', () => {
        const editor = makeEditor()
        editor.commands.setContent('Hi [[@u1]]', { contentType: 'markdown' as never })
        expect(editor.getMarkdown().trim()).toBe('Hi [[@u1]]')
    })

    it('converts several mentions in one body', () => {
        const editor = makeEditor()
        editor.commands.setContent('[[@u1|Ada]] and [[@u2|Grace]]', {
            contentType: 'markdown' as never,
        })
        const md = editor.getMarkdown()
        expect(md).toContain('[[@u1|Ada]]')
        expect(md).toContain('[[@u2|Grace]]')
    })

    it('leaves ordinary text alone', () => {
        const editor = makeEditor()
        editor.commands.setContent('no mentions here', { contentType: 'markdown' as never })
        expect(editor.getMarkdown().trim()).toBe('no mentions here')
    })
})
