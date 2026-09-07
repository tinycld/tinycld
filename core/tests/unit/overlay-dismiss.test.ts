// @vitest-environment happy-dom

import { type LayerRecord, layerToDismiss } from '@tinycld/core/ui/overlay/layer-stack'
import { describe, expect, it } from 'vitest'

function layer(id: number, nodes: Node[], options: Partial<LayerRecord> = {}): LayerRecord {
    return {
        id,
        nodes: () => nodes,
        onDismiss: () => {},
        dismissOnOutside: true,
        dismissOnEscape: true,
        ...options,
    }
}

function element(): HTMLElement {
    const node = document.createElement('div')
    document.body.appendChild(node)
    return node
}

/**
 * One pointerdown dismisses at most the TOP layer, and only when it lands
 * outside that layer's surface and anchor. Everything underneath is left for
 * the next press. This is the rule every surface shares; pinning it here is
 * what lets a menu inside a dialog inside a dialog close one step at a time.
 */
describe('layerToDismiss', () => {
    it('dismisses nothing with no layers', () => {
        expect(layerToDismiss([], element())).toBeNull()
    })

    it('dismisses the top layer on a press outside it', () => {
        const dialog = element()
        const top = layer(1, [dialog])
        expect(layerToDismiss([top], element())).toBe(top)
    })

    it('keeps the top layer on a press inside its surface', () => {
        const dialog = element()
        const inner = document.createElement('button')
        dialog.appendChild(inner)
        expect(layerToDismiss([layer(1, [dialog])], inner)).toBeNull()
    })

    it('keeps the top layer on a press on its anchor, so the trigger toggles it', () => {
        const trigger = element()
        const menu = element()
        expect(layerToDismiss([layer(1, [menu, trigger])], trigger)).toBeNull()
    })

    it('dismisses only the menu on a press inside the dialog beneath it', () => {
        const dialog = element()
        const menu = element()
        const dialogLayer = layer(1, [dialog])
        const menuLayer = layer(2, [menu])
        expect(layerToDismiss([dialogLayer, menuLayer], dialog)).toBe(menuLayer)
    })

    it('never dismisses a layer that opted out of outside dismissal', () => {
        const dialog = element()
        expect(
            layerToDismiss([layer(1, [dialog], { dismissOnOutside: false })], element())
        ).toBeNull()
    })

    it('tolerates a layer whose nodes have not mounted yet', () => {
        const top = layer(1, [null as unknown as Node])
        expect(layerToDismiss([top], element())).toBe(top)
    })
})
