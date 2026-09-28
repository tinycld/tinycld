// @vitest-environment happy-dom

import { cleanup, render as renderBare } from '@testing-library/react'
import { useLayerFocus } from '@tinycld/core/ui/overlay'
import { useState } from 'react'
import { afterEach, describe, expect, it } from 'vitest'

/**
 * A layer that takes focus while open and gives it back when it closes —
 * the shape every Menu and Dialog uses through useLayerFocus.
 */
function Layer({ isActive }: { isActive: boolean }) {
    const [container, setContainer] = useState<HTMLElement | null>(null)
    useLayerFocus({ isActive, container, trap: false })
    if (!isActive) return null
    return (
        <div ref={setContainer as (el: HTMLDivElement | null) => void}>
            <button type="button">Row</button>
        </div>
    )
}

function Harness({ isActive }: { isActive: boolean }) {
    return (
        <>
            <button type="button" id="trigger">
                Trigger
            </button>
            <input id="elsewhere" />
            <Layer isActive={isActive} />
        </>
    )
}

const byId = <T extends HTMLElement>(id: string) => document.getElementById(id) as T

// The restore is deferred to the next animation frame — see the test below.
const nextFrame = () => new Promise<void>(resolve => requestAnimationFrame(() => resolve()))

describe('useLayerFocus — handing focus back on close', () => {
    afterEach(cleanup)

    // The restore is deferred one frame: a modal layer holds the rest of the
    // app `inert` while open, and the re-render that drops it commits AFTER
    // this cleanup runs. Focusing a still-inert element silently does nothing
    // and focus falls to the body, so the restore waits for the removal.
    it('returns focus to the trigger when nothing else claimed it', async () => {
        const { rerender } = renderBare(<Harness isActive={false} />)
        byId('trigger').focus()

        rerender(<Harness isActive={true} />)
        expect(document.activeElement?.textContent).toBe('Row')

        rerender(<Harness isActive={false} />)
        await nextFrame()
        expect(document.activeElement).toBe(byId('trigger'))
    })

    // The reason the restore is deferred at all. While a modal layer is open
    // the rest of the app is `inert`, and `HTMLElement.focus()` on an inert
    // element silently does nothing. The re-render that drops the attribute
    // commits AFTER this cleanup, so restoring inline left focus on the body.
    it('restores focus after the inert attribute is dropped, not before', async () => {
        const { rerender } = renderBare(<Harness isActive={false} />)
        const trigger = byId('trigger')
        trigger.focus()

        rerender(<Harness isActive={true} />)
        // What a modal layer does to everything behind it.
        trigger.parentElement?.setAttribute('inert', '')

        rerender(<Harness isActive={false} />)
        // The attribute comes off in the commit that follows the cleanup.
        trigger.parentElement?.removeAttribute('inert')

        await nextFrame()
        expect(document.activeElement).toBe(trigger)
    })

    it('leaves focus alone when the closing action moved it somewhere new', () => {
        const { rerender } = renderBare(<Harness isActive={false} />)
        byId('trigger').focus()
        rerender(<Harness isActive={true} />)

        // What a menu row that acts by focusing a new field does: the field
        // takes focus BEFORE the layer unmounts. Restoring here would blur it
        // one frame after it appeared — the board-header rename bug.
        byId<HTMLInputElement>('elsewhere').focus()
        rerender(<Harness isActive={false} />)

        expect(document.activeElement).toBe(byId('elsewhere'))
    })
})
