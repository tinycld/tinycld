// @vitest-environment happy-dom

import { renderHook } from '@testing-library/react'
import {
    modalLayerCount,
    resetLayers,
    useIsModalLayerOpen,
    useOverlayLayer,
    wasConsumedByLayerDismissal,
} from '@tinycld/core/ui/overlay/layer-stack'
import { afterEach, describe, expect, it } from 'vitest'

/**
 * A modal layer takes the app behind it out of play. `aria-modal` on the
 * dialog's own surface does not do that — it is a hint to a reader and
 * nothing more — so the app root goes `inert` instead, driven by this count.
 *
 * What the count has to get right is that only dialogs and sheets raise it: a
 * menu or a popover deliberately leaves the page behind usable, and marking
 * one modal would make the page unusable while any menu is open.
 */
afterEach(() => {
    resetLayers()
})

function openLayer(isModal: boolean) {
    return renderHook(() =>
        useOverlayLayer({ isOpen: true, nodes: () => [], onDismiss: () => {}, isModal })
    )
}

describe('modal layers', () => {
    it('counts nothing while no layer is open', () => {
        expect(modalLayerCount()).toBe(0)
    })

    it('counts a modal layer while it is open, and not after', () => {
        const view = openLayer(true)
        expect(modalLayerCount()).toBe(1)
        view.unmount()
        expect(modalLayerCount()).toBe(0)
    })

    it('does not count a non-modal layer', () => {
        openLayer(false)
        expect(modalLayerCount()).toBe(0)
    })

    // A menu opened from inside a dialog must not un-inert the app: the
    // dialog is still modal underneath it.
    it('keeps counting the dialog while a menu is open above it', () => {
        openLayer(true)
        openLayer(false)
        expect(modalLayerCount()).toBe(1)
    })

    it('counts nested modal layers separately', () => {
        openLayer(true)
        const inner = openLayer(true)
        expect(modalLayerCount()).toBe(2)
        inner.unmount()
        expect(modalLayerCount()).toBe(1)
    })
})

describe('useIsModalLayerOpen', () => {
    it('is false with nothing open and true once a modal layer opens', () => {
        const probe = renderHook(() => useIsModalLayerOpen())
        expect(probe.result.current).toBe(false)

        const dialog = openLayer(true)
        probe.rerender()
        expect(probe.result.current).toBe(true)

        dialog.unmount()
        probe.rerender()
        expect(probe.result.current).toBe(false)
    })

    it('stays false for a menu', () => {
        const probe = renderHook(() => useIsModalLayerOpen())
        openLayer(false)
        probe.rerender()
        expect(probe.result.current).toBe(false)
    })
})

/**
 * A dialog's backdrop must not act on the click that follows the press which
 * closed a menu drawn over it. One press produces a capture-phase pointerdown
 * (which dismisses the menu) and then a click on the backdrop, by which point
 * the dialog is topmost again — and closing on it would dismiss two layers
 * with one press, discarding whatever the user had entered in the dialog.
 */
describe('wasConsumedByLayerDismissal', () => {
    it('is false before any layer has been dismissed', () => {
        expect(wasConsumedByLayerDismissal({ pointerId: 7 })).toBe(false)
    })

    it('is true for the press that dismissed a layer', () => {
        const menu = renderHook(() =>
            useOverlayLayer({ isOpen: true, nodes: () => [], onDismiss: () => {} })
        )
        document.body.dispatchEvent(
            new PointerEvent('pointerdown', { bubbles: true, pointerId: 7 })
        )
        expect(wasConsumedByLayerDismissal({ pointerId: 7 })).toBe(true)
        menu.unmount()
    })

    // The guard has to be about THIS press, not about "a dismissal happened
    // recently" — otherwise a genuine second press on the backdrop, the one
    // that should close the dialog, would be swallowed too.
    it('is false for a different press', () => {
        const menu = renderHook(() =>
            useOverlayLayer({ isOpen: true, nodes: () => [], onDismiss: () => {} })
        )
        document.body.dispatchEvent(
            new PointerEvent('pointerdown', { bubbles: true, pointerId: 7 })
        )
        expect(wasConsumedByLayerDismissal({ pointerId: 8 })).toBe(false)
        menu.unmount()
    })
})
