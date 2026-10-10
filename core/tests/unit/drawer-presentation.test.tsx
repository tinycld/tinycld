// @vitest-environment happy-dom

import { OverlayProvider as PortalHost } from '@gluestack-ui/core/overlay/creator'
import { cleanup, fireEvent, render as renderBare } from '@testing-library/react'
import {
    Drawer,
    DrawerBackdrop,
    DrawerBody,
    DrawerCloseButton,
    DrawerContent,
    DrawerHeader,
} from '@tinycld/core/ui/drawer'
import { OverlayProvider } from '@tinycld/core/ui/overlay'
import type { ReactElement } from 'react'
import { Text } from 'react-native'
import { afterEach, describe, expect, it, vi } from 'vitest'

// An overlay drawer renders through gluestack's portal host, which the app
// mounts once (GluestackUIProvider).
const render = (ui: ReactElement) =>
    renderBare(
        <OverlayProvider>
            <PortalHost>{ui}</PortalHost>
        </OverlayProvider>
    )

// The stub renders accessibilityLabel as a plain attribute, not aria-label.
const CLOSE_SELECTOR = '[accessibilitylabel="Close panel"]'

function panel({
    presentation,
    isOpen = true,
    onClose = () => {},
}: {
    presentation: 'overlay' | 'inline'
    isOpen?: boolean
    onClose?: () => void
}) {
    return (
        <Drawer isOpen={isOpen} onClose={onClose} anchor="right" presentation={presentation}>
            <DrawerBackdrop testID="backdrop" />
            <DrawerContent>
                <DrawerHeader>
                    <Text>Title</Text>
                    <DrawerCloseButton accessibilityLabel="Close panel">
                        <Text>x</Text>
                    </DrawerCloseButton>
                </DrawerHeader>
                <DrawerBody>
                    <Text>panel body</Text>
                </DrawerBody>
            </DrawerContent>
        </Drawer>
    )
}

function pressEscape() {
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
}

describe('Drawer presentation', () => {
    afterEach(cleanup)

    it('renders an inline drawer in place, without a backdrop', () => {
        const { container, getByText, queryByTestId } = renderBare(
            <div data-testid="row">{panel({ presentation: 'inline' })}</div>
        )
        expect(getByText('panel body')).not.toBeNull()
        expect(container.querySelector('[data-testid="row"]')?.textContent).toContain('panel body')
        expect(queryByTestId('backdrop')).toBeNull()
    })

    it('renders nothing for a closed inline drawer', () => {
        const { queryByText } = renderBare(panel({ presentation: 'inline', isOpen: false }))
        expect(queryByText('panel body')).toBeNull()
    })

    it('closes an inline drawer from its close button', () => {
        const onClose = vi.fn()
        const { container } = renderBare(panel({ presentation: 'inline', onClose }))
        const button = container.querySelector(CLOSE_SELECTOR)
        if (!button) throw new Error('no close button rendered')
        fireEvent.click(button)
        expect(onClose).toHaveBeenCalledTimes(1)
    })

    it('leaves Escape to the page beside an inline drawer', () => {
        const onClose = vi.fn()
        renderBare(panel({ presentation: 'inline', onClose }))
        pressEscape()
        expect(onClose).not.toHaveBeenCalled()
    })

    it('still closes an overlay drawer from its close button and on Escape', () => {
        const onClose = vi.fn()
        const { getByText } = render(panel({ presentation: 'overlay', onClose }))
        expect(getByText('panel body')).not.toBeNull()
        const button = document.querySelector(CLOSE_SELECTOR)
        if (!button) throw new Error('no close button rendered')
        fireEvent.click(button)
        expect(onClose).toHaveBeenCalledTimes(1)
        pressEscape()
        expect(onClose).toHaveBeenCalledTimes(2)
    })
})
