// @vitest-environment happy-dom

import { cleanup, render as renderBare } from '@testing-library/react'
import { Dialog, type DialogPresentation } from '@tinycld/core/ui/dialog'
import { OverlayProvider } from '@tinycld/core/ui/overlay'
import { Text } from 'react-native'
import { afterEach, describe, expect, it, vi } from 'vitest'

const nextFrame = () => new Promise<void>(resolve => requestAnimationFrame(() => resolve()))

function Harness({
    isOpen,
    presentation,
    onRestoreFocus,
}: {
    isOpen: boolean
    presentation: DialogPresentation
    onRestoreFocus: () => void
}) {
    return (
        <OverlayProvider>
            <Dialog
                isOpen={isOpen}
                onClose={() => {}}
                title="Send"
                presentation={presentation}
                onRestoreFocus={onRestoreFocus}
            >
                <Dialog.Body>
                    <Text>fields</Text>
                </Dialog.Body>
            </Dialog>
        </OverlayProvider>
    )
}

// The opener of a dialog may be gone when it closes (an action that
// re-renders the screen), so the owner moves the focus through
// `onRestoreFocus`, which the dialog calls where it would restore focus.
describe('Dialog onRestoreFocus', () => {
    afterEach(cleanup)

    it.each<DialogPresentation>([
        'dialog',
        'sheet',
    ])('calls onRestoreFocus after a %s closes', async presentation => {
        const onRestoreFocus = vi.fn()
        const { rerender } = renderBare(
            <Harness isOpen presentation={presentation} onRestoreFocus={onRestoreFocus} />
        )
        await nextFrame()
        expect(onRestoreFocus).not.toHaveBeenCalled()

        rerender(
            <Harness isOpen={false} presentation={presentation} onRestoreFocus={onRestoreFocus} />
        )
        await nextFrame()
        expect(onRestoreFocus).toHaveBeenCalledTimes(1)
    })
})
