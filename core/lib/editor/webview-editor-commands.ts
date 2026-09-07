import { type EditorMessage, makeMessage } from './message-bus/types'
import type { EditorCommands } from './types'

/**
 * Posts one message to the in-WebView editor. Returns whether a page existed
 * to receive it; a command sent while none does is simply dropped, which is
 * what a toolbar tap during a page boot should do.
 */
export type PostEditorMessage = (message: EditorMessage) => boolean

/**
 * Builds the EditorCommands object useWebViewEditor returns.
 *
 * Every command is one flat `{namespace: 'format', type, payload}` message;
 * the page's format dispatcher (core's `dispatchFormatAction`, text's
 * `installFormatBridge`) turns each into the matching Tiptap chain. Payload
 * conventions are the wire's: some types carry a bare scalar
 * (`toggle-heading`'s level, `set-text-align`'s string), others an object.
 *
 * Lives in its own module so it can be unit-tested against a recording poster
 * rather than the hook that renders the native host.
 */
export function buildWebViewEditorCommands(post: PostEditorMessage): EditorCommands {
    const send = (type: string, payload: unknown = null) => {
        post(makeMessage('format', type, payload))
    }
    return {
        toggleBold: () => send('toggle-bold'),
        toggleItalic: () => send('toggle-italic'),
        toggleUnderline: () => send('toggle-underline'),
        // camelCase list names are the wire spelling both pages switch on.
        toggleBulletList: () => send('toggle-bulletList'),
        toggleOrderedList: () => send('toggle-orderedList'),
        toggleBlockquote: () => send('toggle-blockquote'),
        toggleHeading: (level: number) => send('toggle-heading', level),
        setLink: (url: string) => send('set-link', url),
        removeLink: () => send('set-link', ''),
        undo: () => send('undo'),
        redo: () => send('redo'),
        insertTable: (rows: number, cols: number) => send('insert-table', { rows, cols }),
        addRowBefore: () => send('add-row-before'),
        addRowAfter: () => send('add-row-after'),
        addColumnBefore: () => send('add-column-before'),
        addColumnAfter: () => send('add-column-after'),
        deleteRow: () => send('delete-row'),
        deleteColumn: () => send('delete-column'),
        deleteTable: () => send('delete-table'),
        mergeCells: () => send('merge-cells'),
        splitCell: () => send('split-cell'),
        mergeOrSplit: () => send('merge-or-split'),
        insertImage: (src: string, alt?: string) => send('insert-image', { src, alt }),
        setCellBorders: (preset, border) => send('set-cell-borders', { preset, border }),
        setCellShading: (color: string | null) => send('set-cell-shading', { color }),
        cut: () => send('cut'),
        copy: () => send('copy'),
        paste: () => send('paste'),
        deleteSelection: () => send('delete-selection'),
        selectAll: () => send('select-all'),
        toggleCode: () => send('toggle-code'),
        toggleCodeBlock: () => send('toggle-code-block'),
        setTextAlign: align => send('set-text-align', align),
        unsetTextAlign: () => send('unset-text-align'),
        indentBlock: () => send('indent-block'),
        outdentBlock: () => send('outdent-block'),
        toggleDropCap: () => send('toggle-drop-cap'),
        setFontSize: (px: number) => send('set-font-size', px),
        unsetFontSize: () => send('unset-font-size'),
        setFontFamily: (family: string) => send('set-font-family', family),
        unsetFontFamily: () => send('unset-font-family'),
        setTextColor: (color: string) => send('set-text-color', color),
        unsetTextColor: () => send('unset-text-color'),
        setBackgroundColor: (color: string) => send('set-background-color', color),
        unsetBackgroundColor: () => send('unset-background-color'),
        updateImageAttrs: payload => send('update-image-attrs', payload),
    }
}
