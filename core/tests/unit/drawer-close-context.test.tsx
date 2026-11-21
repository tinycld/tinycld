// @vitest-environment happy-dom

import { OverlayProvider as PortalHost } from '@gluestack-ui/core/overlay/creator'
import { cleanup, fireEvent, render as renderBare } from '@testing-library/react'
import { Drawer, DrawerBody, DrawerContent } from '@tinycld/core/ui/drawer'
import { useDrawerClose } from '@tinycld/core/ui/drawer/overlay-context'
import { OverlayProvider } from '@tinycld/core/ui/overlay'
import type { ReactElement } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

// An overlay drawer renders through gluestack's portal host, which the app
// mounts once (GluestackUIProvider). The portal re-renders the content at the
// host, so a context set around the drawer's root does not reach its content.
const render = (ui: ReactElement) =>
    renderBare(
        <OverlayProvider>
            <PortalHost>{ui}</PortalHost>
        </OverlayProvider>
    )

// Calls the close callback the drawer content sees (the one swipe-to-dismiss
// uses), so the test observes which callback reached the portalled subtree.
function CloseProbe() {
    const close = useDrawerClose()
    return (
        <button type="button" data-testid="close-probe" onClick={() => close?.()}>
            {close ? 'has-close' : 'no-close'}
        </button>
    )
}

function panel(presentation: 'overlay' | 'inline', onClose: () => void) {
    return (
        <Drawer isOpen onClose={onClose} anchor="right" presentation={presentation}>
            <DrawerContent>
                <DrawerBody>
                    <CloseProbe />
                </DrawerBody>
            </DrawerContent>
        </Drawer>
    )
}

function pressProbe() {
    const probe = document.querySelector<HTMLElement>('[data-testid="close-probe"]')
    if (!probe) throw new Error('close probe not rendered')
    fireEvent.click(probe)
    return probe
}

describe('Drawer close callback', () => {
    afterEach(cleanup)

    it("hands an overlay drawer's onClose to its portalled content", () => {
        const onClose = vi.fn()
        render(panel('overlay', onClose))
        const probe = pressProbe()
        expect(probe.textContent).toBe('has-close')
        expect(onClose).toHaveBeenCalledTimes(1)
    })

    it("hands an inline drawer's onClose to its content", () => {
        const onClose = vi.fn()
        renderBare(panel('inline', onClose))
        const probe = pressProbe()
        expect(probe.textContent).toBe('has-close')
        expect(onClose).toHaveBeenCalledTimes(1)
    })
})
