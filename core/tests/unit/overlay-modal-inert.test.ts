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
    function pressOn(target: EventTarget, pointerId: number) {
        target.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true, pointerId }))
    }

    it('is false before any layer has been dismissed', () => {
        expect(wasConsumedByLayerDismissal({ pointerId: 1 })).toBe(false)
    })

    it('is true for the press that dismissed a layer', () => {
        const menu = renderHook(() =>
            useOverlayLayer({ isOpen: true, nodes: () => [], onDismiss: () => {} })
        )
        pressOn(document.body, 1)
        expect(wasConsumedByLayerDismissal({ pointerId: 1 })).toBe(true)
        menu.unmount()
    })

    /**
     * The case a real mouse produces, and the one the first version of this
     * guard got wrong. A mouse reuses pointerId 1 for its entire life, so a
     * flag that was only ever set — never cleared — left the dialog's backdrop
     * permanently dead after the first menu dismissal.
     *
     * Press 1 closes the menu, and the click it produces on the backdrop is
     * swallowed. Press 2, same pointerId, has dismissed nothing, so its click
     * must close the dialog.
     */
    it('swallows only the click belonging to the dismissing press, same pointerId', () => {
        const menu = renderHook(() =>
            useOverlayLayer({ isOpen: true, nodes: () => [], onDismiss: () => {} })
        )

        // Press 1: outside the menu, so it dismisses it. The backdrop click
        // that follows belongs to this press and must not close the dialog.
        pressOn(document.body, 1)
        expect(wasConsumedByLayerDismissal({ pointerId: 1 })).toBe(true)
        menu.unmount()

        // Press 2: the menu is gone, so nothing is dismissed. Same mouse,
        // same pointerId — this click MUST reach the dialog.
        pressOn(document.body, 1)
        expect(wasConsumedByLayerDismissal({ pointerId: 1 })).toBe(false)
    })

    // Reading it consumes it: one press yields one swallowed click, not a
    // standing veto on every click that follows.
    it('consumes the flag on read', () => {
        const menu = renderHook(() =>
            useOverlayLayer({ isOpen: true, nodes: () => [], onDismiss: () => {} })
        )
        pressOn(document.body, 1)
        expect(wasConsumedByLayerDismissal({ pointerId: 1 })).toBe(true)
        expect(wasConsumedByLayerDismissal({ pointerId: 1 })).toBe(false)
        menu.unmount()
    })

    it('is false for a different pointer', () => {
        const menu = renderHook(() =>
            useOverlayLayer({ isOpen: true, nodes: () => [], onDismiss: () => {} })
        )
        pressOn(document.body, 7)
        expect(wasConsumedByLayerDismissal({ pointerId: 8 })).toBe(false)
        menu.unmount()
    })

    // An event with no pointerId cannot be tied to the press that dismissed
    // the layer. Treating two absent ids as equal would be the same
    // permanently-dead-backdrop bug by another route.
    it('never treats an absent pointer id as consumed', () => {
        const menu = renderHook(() =>
            useOverlayLayer({ isOpen: true, nodes: () => [], onDismiss: () => {} })
        )
        document.body.dispatchEvent(new Event('pointerdown', { bubbles: true }))
        expect(wasConsumedByLayerDismissal(undefined)).toBe(false)
        expect(wasConsumedByLayerDismissal({})).toBe(false)
        menu.unmount()
    })

    // A press that dismisses nothing must leave no residue behind. The press
    // lands INSIDE the layer here, so the stack dismisses nothing and the
    // flag has to come back down on its own.
    it('clears the flag on a press that dismisses nothing', () => {
        const inside = document.createElement('div')
        document.body.appendChild(inside)
        const menu = renderHook(() =>
            useOverlayLayer({ isOpen: true, nodes: () => [inside], onDismiss: () => {} })
        )

        pressOn(document.body, 1)
        expect(wasConsumedByLayerDismissal({ pointerId: 1 })).toBe(true)

        // Inside the layer: nothing is dismissed, so the next click is free.
        pressOn(inside, 1)
        expect(wasConsumedByLayerDismissal({ pointerId: 1 })).toBe(false)

        menu.unmount()
        inside.remove()
    })
})
