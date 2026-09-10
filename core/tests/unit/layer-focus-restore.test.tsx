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

describe('useLayerFocus — handing focus back on close', () => {
    afterEach(cleanup)

    it('returns focus to the trigger when nothing else claimed it', () => {
        const { rerender } = renderBare(<Harness isActive={false} />)
        byId('trigger').focus()

        rerender(<Harness isActive={true} />)
        expect(document.activeElement?.textContent).toBe('Row')

        rerender(<Harness isActive={false} />)
        expect(document.activeElement).toBe(byId('trigger'))
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
