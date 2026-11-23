// @vitest-environment happy-dom

import { cleanup, render as renderBare } from '@testing-library/react'
import { useLayerFocus } from '@tinycld/core/ui/overlay'
import { useState } from 'react'
import { Platform } from 'react-native'
import { afterEach, describe, expect, it, vi } from 'vitest'

/**
 * A layer whose opener may be gone by the time it closes (a dialog whose
 * action re-renders the screen), so the owner moves the focus itself
 * through `onRestoreFocus`.
 */
function Layer({ isActive, onRestoreFocus }: { isActive: boolean; onRestoreFocus?: () => void }) {
    const [container, setContainer] = useState<HTMLElement | null>(null)
    useLayerFocus({ isActive, container, trap: false, onRestoreFocus })
    if (!isActive) return null
    return (
        <div ref={setContainer as (el: HTMLDivElement | null) => void}>
            <button type="button">Row</button>
        </div>
    )
}

function Harness({
    isActive,
    hasOpener = true,
    onRestoreFocus,
}: {
    isActive: boolean
    hasOpener?: boolean
    onRestoreFocus?: () => void
}) {
    const opener = hasOpener ? (
        <button type="button" id="opener">
            Opener
        </button>
    ) : null
    return (
        <>
            {opener}
            <input id="elsewhere" />
            <Layer isActive={isActive} onRestoreFocus={onRestoreFocus} />
        </>
    )
}

const byId = <T extends HTMLElement>(id: string) => document.getElementById(id) as T
const nextFrame = () => new Promise<void>(resolve => requestAnimationFrame(() => resolve()))
const originalOS = Platform.OS

describe('useLayerFocus — onRestoreFocus', () => {
    afterEach(() => {
        cleanup()
        Platform.OS = originalOS
    })

    it('calls onRestoreFocus one frame after close, in place of the default restore', async () => {
        const onRestoreFocus = vi.fn(() => byId('elsewhere').focus())
        const { rerender } = renderBare(
            <Harness isActive={false} onRestoreFocus={onRestoreFocus} />
        )
        byId('opener').focus()
        rerender(<Harness isActive={true} onRestoreFocus={onRestoreFocus} />)

        rerender(<Harness isActive={false} onRestoreFocus={onRestoreFocus} />)
        // The same one-frame wait as the default restore.
        expect(onRestoreFocus).not.toHaveBeenCalled()
        await nextFrame()
        expect(onRestoreFocus).toHaveBeenCalledTimes(1)
        expect(document.activeElement).toBe(byId('elsewhere'))
    })

    it('is called when the opener is gone', async () => {
        const onRestoreFocus = vi.fn(() => byId('elsewhere').focus())
        const { rerender } = renderBare(
            <Harness isActive={false} onRestoreFocus={onRestoreFocus} />
        )
        byId('opener').focus()
        rerender(<Harness isActive={true} onRestoreFocus={onRestoreFocus} />)

        // The action that closes the dialog also removes its opener.
        rerender(<Harness isActive={false} hasOpener={false} onRestoreFocus={onRestoreFocus} />)
        await nextFrame()
        expect(onRestoreFocus).toHaveBeenCalledTimes(1)
        expect(document.activeElement).toBe(byId('elsewhere'))
    })

    it('is not called when the closing action moved the focus itself', async () => {
        const onRestoreFocus = vi.fn()
        const { rerender } = renderBare(
            <Harness isActive={false} onRestoreFocus={onRestoreFocus} />
        )
        byId('opener').focus()
        rerender(<Harness isActive={true} onRestoreFocus={onRestoreFocus} />)

        byId<HTMLInputElement>('elsewhere').focus()
        rerender(<Harness isActive={false} onRestoreFocus={onRestoreFocus} />)
        await nextFrame()
        expect(onRestoreFocus).not.toHaveBeenCalled()
    })

    it('is called on native when the layer closes', async () => {
        Platform.OS = 'ios'
        const onRestoreFocus = vi.fn()
        const { rerender } = renderBare(
            <Harness isActive={false} onRestoreFocus={onRestoreFocus} />
        )
        rerender(<Harness isActive={true} onRestoreFocus={onRestoreFocus} />)
        expect(onRestoreFocus).not.toHaveBeenCalled()

        rerender(<Harness isActive={false} onRestoreFocus={onRestoreFocus} />)
        await nextFrame()
        expect(onRestoreFocus).toHaveBeenCalledTimes(1)
    })
})
