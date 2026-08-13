// @vitest-environment happy-dom
//
// The provider decides `inInput` per key event; a DOM is what it inspects.
import { isInputKeyEvent } from '@tinycld/core/lib/shortcuts/provider.web'
import { describe, expect, it } from 'vitest'

function keyEventOn(target: EventTarget, key = 'Escape'): KeyboardEvent {
    const event = new KeyboardEvent('keydown', { key, bubbles: true })
    Object.defineProperty(event, 'target', { value: target })
    return event
}

describe('isInputKeyEvent', () => {
    it('is true for a key typed into an input', () => {
        const input = document.createElement('input')
        document.body.appendChild(input)
        expect(isInputKeyEvent(keyEventOn(input))).toBe(true)
        input.remove()
    })

    it('is true for a key typed into a ProseMirror surface', () => {
        const editor = document.createElement('div')
        editor.className = 'ProseMirror'
        const inner = document.createElement('p')
        editor.appendChild(inner)
        document.body.appendChild(editor)
        expect(isInputKeyEvent(keyEventOn(inner))).toBe(true)
        editor.remove()
    })

    it('stays true when the editor was unmounted before the event reached the window', () => {
        // The regression: an editor whose Escape handler unmounts (or blurs)
        // it. React flushes that update while the event is still bubbling, so
        // at the window listener `document.activeElement` is already BODY —
        // but the TARGET still names the node the key was typed into, even
        // detached. Deciding from activeElement made the peek's Escape fire
        // on the very keystroke the editor had handled.
        const editor = document.createElement('div')
        editor.className = 'ProseMirror'
        const inner = document.createElement('p')
        editor.appendChild(inner)
        document.body.appendChild(editor)
        const event = keyEventOn(inner)
        editor.remove()
        expect(document.activeElement?.tagName).toBe('BODY')
        expect(isInputKeyEvent(event)).toBe(true)
    })

    it('is true for a detached contenteditable, where isContentEditable reads false', () => {
        const editable = document.createElement('div')
        editable.setAttribute('contenteditable', 'true')
        const event = keyEventOn(editable)
        expect(isInputKeyEvent(event)).toBe(true)
    })

    it('is false for a key pressed over plain content', () => {
        const div = document.createElement('div')
        document.body.appendChild(div)
        expect(isInputKeyEvent(keyEventOn(div))).toBe(false)
        div.remove()
    })

    it('falls back to the focused element when the target is not an input', () => {
        const input = document.createElement('input')
        document.body.appendChild(input)
        input.focus()
        expect(isInputKeyEvent(keyEventOn(document.body))).toBe(true)
        input.remove()
    })
})
