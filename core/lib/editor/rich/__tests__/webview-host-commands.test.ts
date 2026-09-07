// @vitest-environment happy-dom
//
// Tiptap needs a DOM to mount an editor; core's suite defaults to node.
import { Editor } from '@tiptap/core'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { makeMessage } from '../../message-bus/types'
import { buildRichEditorExtensions } from '../extensions'
import { dispatchFormatAction } from '../webview/source/Editor'
import {
    applyKeyboardInset,
    focusEditor,
    readFocusPosition,
    readKeyboardInset,
} from '../webview/source/host-commands'

/**
 * The host's instructions to the page, driven against a real Tiptap instance:
 * the flat format messages the toolbar now sends, and the 'app' messages that
 * replaced what TenTap's host did through its own bridge.
 */

let editor: Editor

beforeEach(() => {
    editor = new Editor({ extensions: buildRichEditorExtensions(), content: '<p>hello world</p>' })
})

afterEach(() => {
    editor.destroy()
})

describe('format dispatch', () => {
    it('applies a flat format message', () => {
        editor.commands.selectAll()
        dispatchFormatAction(editor, makeMessage('format', 'toggle-bold', null))
        expect(editor.isActive('bold')).toBe(true)
    })

    it('reads the heading level from a bare payload', () => {
        dispatchFormatAction(editor, makeMessage('format', 'toggle-heading', 2))
        expect(editor.isActive('heading', { level: 2 })).toBe(true)
    })

    it('honours set-editable', () => {
        dispatchFormatAction(editor, makeMessage('format', 'set-editable', false))
        expect(editor.isEditable).toBe(false)
        dispatchFormatAction(editor, makeMessage('format', 'set-editable', true))
        expect(editor.isEditable).toBe(true)
    })

    it('ignores a set-editable that is not a boolean', () => {
        dispatchFormatAction(editor, makeMessage('format', 'set-editable', 'no'))
        expect(editor.isEditable).toBe(true)
    })
})

describe('focus', () => {
    it('reads the three focus payload shapes and falls back to the end', () => {
        expect(readFocusPosition('start')).toBe('start')
        expect(readFocusPosition({ x: 3, y: 4 })).toEqual({ x: 3, y: 4 })
        expect(readFocusPosition({ x: 'a' })).toBe('end')
        expect(readFocusPosition(undefined)).toBe('end')
    })

    it('moves the selection to the document start', () => {
        focusEditor(editor, 'start')
        expect(editor.state.selection.from).toBe(1)
        focusEditor(editor, 'end')
        expect(editor.state.selection.from).toBe(editor.state.doc.content.size - 1)
    })

    it('falls back to the end for a point outside any text', () => {
        // happy-dom does no layout, so every point misses — the same outcome
        // as a press in the padding on a device.
        focusEditor(editor, { x: -100, y: -100 })
        expect(editor.state.selection.from).toBe(editor.state.doc.content.size - 1)
    })
})

describe('keyboard inset', () => {
    it('reads a non-negative bottom and rejects anything else', () => {
        expect(readKeyboardInset({ bottom: 310 })).toBe(310)
        expect(readKeyboardInset({ bottom: -1 })).toBeNull()
        expect(readKeyboardInset({})).toBeNull()
        expect(readKeyboardInset(null)).toBeNull()
    })

    it('pads the document and moves the scroll margin', () => {
        applyKeyboardInset(editor, 310)
        expect((editor.view.dom as HTMLElement).style.paddingBottom).toBe('310px')
        const margin = editor.options.editorProps?.scrollMargin
        expect(margin).toEqual({ top: 0, right: 0, bottom: 310, left: 0 })
        applyKeyboardInset(editor, 0)
        expect((editor.view.dom as HTMLElement).style.paddingBottom).toBe('0px')
    })
})
