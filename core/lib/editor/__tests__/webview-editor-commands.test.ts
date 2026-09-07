import { describe, expect, it, vi } from 'vitest'
import type { EditorMessage } from '../message-bus/types'
import { buildWebViewEditorCommands } from '../webview-editor-commands'

/**
 * Every command is one flat format message. The pages' dispatchers switch on
 * these exact type strings and payload shapes, so a drift here is a toolbar
 * button that silently no-ops on device — pin them.
 */
function record() {
    const sent: EditorMessage[] = []
    const commands = buildWebViewEditorCommands(message => {
        sent.push(message)
        return true
    })
    return { sent, commands }
}

describe('buildWebViewEditorCommands', () => {
    it('sends the basic formatting commands as flat format messages', () => {
        const { sent, commands } = record()
        commands.toggleBold()
        commands.toggleItalic()
        commands.toggleUnderline()
        commands.toggleBulletList()
        commands.toggleOrderedList()
        commands.toggleBlockquote()
        commands.undo()
        commands.redo()
        expect(sent).toEqual([
            { namespace: 'format', type: 'toggle-bold', payload: null },
            { namespace: 'format', type: 'toggle-italic', payload: null },
            { namespace: 'format', type: 'toggle-underline', payload: null },
            { namespace: 'format', type: 'toggle-bulletList', payload: null },
            { namespace: 'format', type: 'toggle-orderedList', payload: null },
            { namespace: 'format', type: 'toggle-blockquote', payload: null },
            { namespace: 'format', type: 'undo', payload: null },
            { namespace: 'format', type: 'redo', payload: null },
        ])
    })

    it('carries the heading level and link href as bare payloads', () => {
        const { sent, commands } = record()
        commands.toggleHeading(2)
        commands.setLink('https://example.com')
        commands.removeLink()
        expect(sent.map(m => [m.type, m.payload])).toEqual([
            ['toggle-heading', 2],
            ['set-link', 'https://example.com'],
            ['set-link', ''],
        ])
    })

    it('sends table, image, clipboard, and style commands with their payloads', () => {
        const { sent, commands } = record()
        commands.insertTable?.(2, 3)
        commands.insertImage?.('/api/files/x.png', 'diagram')
        commands.setCellBorders?.('outer', { widthPx: 2 })
        commands.setCellShading?.('#ffff00')
        commands.setTextAlign?.('center')
        commands.setFontSize?.(14)
        commands.setFontFamily?.('Georgia')
        commands.setTextColor?.('#ff0000')
        commands.updateImageAttrs?.({ wrap: 'left', width: 200 })
        commands.cut?.()
        commands.mergeOrSplit?.()
        expect(sent.map(m => [m.type, m.payload])).toEqual([
            ['insert-table', { rows: 2, cols: 3 }],
            ['insert-image', { src: '/api/files/x.png', alt: 'diagram' }],
            ['set-cell-borders', { preset: 'outer', border: { widthPx: 2 } }],
            ['set-cell-shading', { color: '#ffff00' }],
            ['set-text-align', 'center'],
            ['set-font-size', 14],
            ['set-font-family', 'Georgia'],
            ['set-text-color', '#ff0000'],
            ['update-image-attrs', { wrap: 'left', width: 200 }],
            ['cut', null],
            ['merge-or-split', null],
        ])
        expect(sent.every(m => m.namespace === 'format')).toBe(true)
    })

    it('tolerates a poster with nothing to post to', () => {
        const post = vi.fn(() => false)
        const commands = buildWebViewEditorCommands(post)
        expect(() => commands.toggleBold()).not.toThrow()
        expect(post).toHaveBeenCalledTimes(1)
    })
})
