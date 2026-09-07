// @vitest-environment happy-dom
//
// Tiptap needs a DOM to mount an editor; core's suite defaults to node.
import { Editor } from '@tiptap/core'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { makeMessage } from '../../message-bus/types'
import { buildRichEditorExtensions } from '../extensions'
import { handleHtmlMessage } from '../webview/source/Editor'
import { HTML_GET, HTML_RESULT, HTML_SET } from '../webview/source/protocol'

/**
 * Drives the page's html handler against a real Tiptap instance — exactly what
 * happens on a `html.set` / `html.get` from the native host. The device path
 * has no other mechanical coverage, and a page that silently ignores a `get`
 * is precisely the bug that made mail's compose close hang.
 */

let editor: Editor | null = null
const postMessage = vi.fn<(s: string) => void>()

beforeEach(() => {
    postMessage.mockReset()
    window.ReactNativeWebView = { postMessage }
    editor = new Editor({ extensions: buildRichEditorExtensions() })
})

afterEach(() => {
    editor?.destroy()
    editor = null
    window.ReactNativeWebView = undefined
})

function posted() {
    return postMessage.mock.calls.map(([raw]) => JSON.parse(raw) as unknown)
}

describe('WebView html channel', () => {
    it('answers a get with the html and text, echoing the requestId', () => {
        if (!editor) throw new Error('editor not mounted')
        editor.commands.setContent('<p>Hello <strong>there</strong></p>')
        handleHtmlMessage(editor, makeMessage('html', HTML_GET, null, 'html-7'), false)
        expect(posted()).toEqual([
            {
                namespace: 'html',
                type: HTML_RESULT,
                requestId: 'html-7',
                payload: { html: '<p>Hello <strong>there</strong></p>', text: 'Hello there' },
            },
        ])
    })

    it('replaces the document on a set', () => {
        if (!editor) throw new Error('editor not mounted')
        handleHtmlMessage(editor, makeMessage('html', HTML_SET, { html: '<p>draft</p>' }), false)
        expect(editor.getText()).toBe('draft')
        handleHtmlMessage(editor, makeMessage('html', HTML_SET, { html: '' }), false)
        expect(editor.getText()).toBe('')
    })

    it('leaves a collaborative document alone on a set', () => {
        if (!editor) throw new Error('editor not mounted')
        editor.commands.setContent('<p>shared</p>')
        handleHtmlMessage(editor, makeMessage('html', HTML_SET, { html: '<p>mine</p>' }), true)
        expect(editor.getText()).toBe('shared')
    })
})
