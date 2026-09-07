// @vitest-environment happy-dom

import { cleanup, fireEvent, render as renderBare } from '@testing-library/react'
import { Dialog } from '@tinycld/core/ui/dialog'
import { OverlayProvider } from '@tinycld/core/ui/overlay'
import type { ReactElement } from 'react'
import { Text } from 'react-native'
import { afterEach, describe, expect, it, vi } from 'vitest'

// Every surface renders through the overlay host, which the app mounts once.
const render = (ui: ReactElement) => renderBare(<OverlayProvider>{ui}</OverlayProvider>)

// The stub renders accessibilityLabel as a plain attribute, not aria-label.
const CLOSE_SELECTOR = '[role="dialog"] [accessibilitylabel="Close"]'
function closeButton(container: HTMLElement): Element {
    const button = container.querySelector(CLOSE_SELECTOR)
    if (!button) throw new Error('no close button rendered')
    return button
}

/**
 * The composite every feature dialog is built from. These cover the shape —
 * header, body, footer, and the close paths — rather than the overlay's
 * positioning, which the modal itself owns.
 */
describe('Dialog', () => {
    afterEach(cleanup)

    it('renders nothing while closed', () => {
        const { queryByText } = render(
            <Dialog isOpen={false} onClose={() => {}} title="Hidden">
                <Dialog.Body>
                    <Text>never</Text>
                </Dialog.Body>
            </Dialog>
        )
        expect(queryByText('Hidden')).toBeNull()
        expect(queryByText('never')).toBeNull()
    })

    it('shows the title, description, body and footer buttons', () => {
        const onClose = vi.fn()
        const onSave = vi.fn()
        const { getByText, container } = render(
            <Dialog isOpen onClose={onClose} title="Board settings" description="Tune the board">
                <Dialog.Body>
                    <Text>fields</Text>
                </Dialog.Body>
                <Dialog.Footer>
                    <Dialog.CancelButton onPress={onClose} />
                    <Dialog.ActionButton label="Save" onPress={onSave} />
                </Dialog.Footer>
            </Dialog>
        )
        expect(getByText('Board settings')).not.toBeNull()
        expect(getByText('Tune the board')).not.toBeNull()
        expect(getByText('fields')).not.toBeNull()

        fireEvent.click(getByText('Save'))
        expect(onSave).toHaveBeenCalledTimes(1)

        fireEvent.click(getByText('Cancel'))
        expect(onClose).toHaveBeenCalledTimes(1)

        fireEvent.click(closeButton(container))
        expect(onClose).toHaveBeenCalledTimes(2)
    })

    it('omits the close button when the dialog must be answered', () => {
        const { container } = render(
            <Dialog isOpen onClose={() => {}} title="Delete?" hasCloseButton={false}>
                <Dialog.Footer>
                    <Dialog.ActionButton label="Delete" onPress={() => {}} isDestructive />
                </Dialog.Footer>
            </Dialog>
        )
        expect(container.querySelector(CLOSE_SELECTOR)).toBeNull()
    })

    it('does not fire a disabled action', () => {
        const onSave = vi.fn()
        const { getByText } = render(
            <Dialog isOpen onClose={() => {}} title="Form">
                <Dialog.Footer>
                    <Dialog.ActionButton label="Save" onPress={onSave} isDisabled />
                </Dialog.Footer>
            </Dialog>
        )
        fireEvent.click(getByText('Save'))
        expect(onSave).not.toHaveBeenCalled()
    })
})
