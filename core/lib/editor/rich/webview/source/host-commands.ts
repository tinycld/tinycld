import type { Editor } from '@tiptap/core'

/**
 * Host → page instructions on the 'app' namespace that act on the editor
 * itself rather than its content: where the caret goes, and how much of the
 * document's bottom the keyboard covers.
 *
 * Pure DOM, no React, so both pages (core's rich editor and text's document
 * editor) call the same functions and a unit test can drive them against a
 * real Tiptap instance.
 */
export type FocusPosition = 'start' | 'end' | { x: number; y: number }

export function readFocusPosition(payload: unknown): FocusPosition {
    if (payload === 'start' || payload === 'end') return payload
    if (typeof payload === 'object' && payload !== null) {
        const { x, y } = payload as { x?: unknown; y?: unknown }
        if (typeof x === 'number' && typeof y === 'number') return { x, y }
    }
    return 'end'
}

/**
 * Put the caret where the host asked. A point is where the user pressed, in
 * viewport coordinates — the read view and the editing surface occupy the
 * same box, so the press on the prose is where the caret belongs. A point that
 * resolves to nothing (padding, an editor not yet laid out) falls back to the
 * end, the previous behaviour.
 */
export function focusEditor(editor: Editor, payload: unknown): void {
    const position = readFocusPosition(payload)
    if (typeof position === 'string') {
        editor.commands.focus(position)
        return
    }
    const hit = editor.view.posAtCoords({ left: position.x, top: position.y })
    editor.commands.focus(hit?.pos ?? 'end')
}

export function readKeyboardInset(payload: unknown): number | null {
    const bottom = (payload as { bottom?: unknown } | null | undefined)?.bottom
    return typeof bottom === 'number' && bottom >= 0 ? bottom : null
}

/**
 * The inset last applied. A hand-off rebuilds the editor while the keyboard
 * may still be up, so the new one starts from this rather than from zero.
 */
let lastInset = 0

/**
 * Pad the document's bottom by the keyboard's height, and tell ProseMirror
 * the same so its scroll-into-view keeps the caret above it. The padding is
 * what TenTap's host injected as raw JavaScript; the scroll threshold is the
 * half it also sent but our pages never received.
 */
export function applyKeyboardInset(editor: Editor, bottom: number): void {
    lastInset = bottom
    ;(editor.view.dom as HTMLElement).style.paddingBottom = `${bottom}px`
    const edges = { top: 0, right: 0, bottom, left: 0 }
    editor.setOptions({
        editorProps: {
            ...editor.options.editorProps,
            scrollThreshold: edges,
            scrollMargin: edges,
        },
    })
}

export function reapplyKeyboardInset(editor: Editor): void {
    if (lastInset > 0) applyKeyboardInset(editor, lastInset)
}
