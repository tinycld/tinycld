// @vitest-environment happy-dom

import { OverlayProvider as PortalHost } from '@gluestack-ui/core/overlay/creator'
import { act, cleanup, fireEvent, render } from '@testing-library/react'
import { Drawer, DrawerBackdrop, DrawerBody, DrawerContent } from '@tinycld/core/ui/drawer'
import { Menu } from '@tinycld/core/ui/menu'
import { OverlayHost, OverlayProvider } from '@tinycld/core/ui/overlay'
import { resetLayers } from '@tinycld/core/ui/overlay/layer-stack'
import type { PopoverPresentation } from '@tinycld/core/ui/popover'
import { Sheet } from '@tinycld/core/ui/sheet'
import { type ReactNode, useState } from 'react'
import { BackHandler, Platform, Text } from 'react-native'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

/**
 * Escape closes only the topmost layer. A Menu or a Sheet opened inside an
 * overlay Drawer sits above the Drawer, so the first Escape closes it and the
 * Drawer stays open; the next Escape closes the Drawer.
 */
function Shell({ children }: { children: ReactNode }) {
    return (
        <OverlayProvider>
            <PortalHost>
                <OverlayHost name="sheet" />
                {children}
            </PortalHost>
        </OverlayProvider>
    )
}

function DrawerAround({
    onClose,
    isOpen = true,
    closeOnOverlayClick,
    children,
}: {
    onClose: () => void
    isOpen?: boolean
    closeOnOverlayClick?: boolean
    children?: ReactNode
}) {
    return (
        <Drawer
            isOpen={isOpen}
            onClose={onClose}
            anchor="right"
            closeOnOverlayClick={closeOnOverlayClick}
        >
            <DrawerBackdrop testID="drawer-backdrop" />
            <DrawerContent>
                <DrawerBody>
                    <Text>drawer body</Text>
                    {children}
                </DrawerBody>
            </DrawerContent>
        </Drawer>
    )
}

function pressEscape() {
    act(() => {
        fireEvent.keyDown(document.body, { key: 'Escape' })
    })
}

function releaseEscape() {
    act(() => {
        document.body.dispatchEvent(new KeyboardEvent('keyup', { key: 'Escape', bubbles: true }))
    })
}

describe('Escape in a layer inside a Drawer', () => {
    afterEach(() => {
        cleanup()
        resetLayers()
    })

    it.each<PopoverPresentation>([
        'sheet',
        'popover',
    ])('closes only the %s menu, then the drawer on the next Escape', presentation => {
        const closeDrawer = vi.fn()
        const changeMenu = vi.fn()
        const menu = (isOpen: boolean) => (
            <Shell>
                <DrawerAround onClose={closeDrawer}>
                    <Menu
                        isOpen={isOpen}
                        onOpenChange={changeMenu}
                        anchor={{ x: 10, y: 10 }}
                        presentation={presentation}
                    >
                        <Menu.Item label="North" onSelect={() => {}} />
                    </Menu>
                </DrawerAround>
            </Shell>
        )
        // The menu opens in a later commit than the drawer, as in the app.
        const { rerender } = render(menu(false))
        rerender(menu(true))

        pressEscape()
        expect(changeMenu).toHaveBeenCalledWith(false)
        expect(closeDrawer).not.toHaveBeenCalled()

        rerender(menu(false))
        pressEscape()
        expect(closeDrawer).toHaveBeenCalledTimes(1)
    })

    it('closes only a sheet opened inside the drawer, then the drawer on the next Escape', () => {
        const closeDrawer = vi.fn()
        const closeSheet = vi.fn()
        const sheet = (isOpen: boolean) => (
            <Shell>
                <DrawerAround onClose={closeDrawer}>
                    <Sheet isOpen={isOpen} onClose={closeSheet} testID="drawer-sheet">
                        <Text>drawer sheet</Text>
                    </Sheet>
                </DrawerAround>
            </Shell>
        )
        const { rerender } = render(sheet(false))
        rerender(sheet(true))

        pressEscape()
        expect(closeSheet).toHaveBeenCalledTimes(1)
        expect(closeDrawer).not.toHaveBeenCalled()

        rerender(sheet(false))
        pressEscape()
        expect(closeDrawer).toHaveBeenCalledTimes(1)
    })

    it('closes only the menu when the drawer re-renders with a new onClose after the menu opens', () => {
        const closeDrawer = vi.fn()
        const opener: { current: ((open: boolean) => void) | null } = { current: null }
        // The menu closes through state, flushed at once: a browser flushes the
        // discrete keydown update between two document listeners, so the menu's
        // layer is gone before a listener added after the stack's runs. The key
        // is dispatched outside act so this act flushes inside the listener.
        function MenuInDrawer({ onClose }: { onClose: () => void }) {
            const [isOpen, setOpen] = useState(false)
            opener.current = setOpen
            const changeMenu = (open: boolean) => {
                act(() => setOpen(open))
            }
            return (
                <Shell>
                    <DrawerAround onClose={onClose}>
                        <Menu
                            isOpen={isOpen}
                            onOpenChange={changeMenu}
                            anchor={{ x: 10, y: 10 }}
                            presentation="popover"
                        >
                            <Menu.Item label="North" onSelect={() => {}} />
                        </Menu>
                    </DrawerAround>
                </Shell>
            )
        }
        const { rerender, queryByText } = render(<MenuInDrawer onClose={() => closeDrawer()} />)
        act(() => opener.current?.(true))
        expect(queryByText('North')).not.toBeNull()
        // A new onClose identity after the menu opened: the drawer's effect runs again.
        rerender(<MenuInDrawer onClose={() => closeDrawer()} />)

        document.body.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
        expect(queryByText('North')).toBeNull()
        expect(closeDrawer).not.toHaveBeenCalled()

        pressEscape()
        expect(closeDrawer).toHaveBeenCalledTimes(1)
    })

    // Gluestack's Overlay listens for an Escape keyup on document.body and
    // closes through its onClose; only the layer stack may close the drawer.
    it('keeps the drawer open on an Escape keyup', () => {
        const closeDrawer = vi.fn()
        render(
            <Shell>
                <DrawerAround onClose={closeDrawer} />
            </Shell>
        )

        releaseEscape()
        expect(closeDrawer).not.toHaveBeenCalled()
    })

    it('keeps the drawer open on the keyup of the Escape that closed a menu in it', () => {
        const closeDrawer = vi.fn()
        const menu = (isOpen: boolean) => (
            <Shell>
                <DrawerAround onClose={closeDrawer}>
                    <Menu
                        isOpen={isOpen}
                        onOpenChange={open => rerender(menu(open))}
                        anchor={{ x: 10, y: 10 }}
                        presentation="popover"
                    >
                        <Menu.Item label="North" onSelect={() => {}} />
                    </Menu>
                </DrawerAround>
            </Shell>
        )
        const { rerender, queryByText } = render(menu(false))
        rerender(menu(true))
        expect(queryByText('North')).not.toBeNull()

        pressEscape()
        expect(queryByText('North')).toBeNull()
        releaseEscape()
        expect(closeDrawer).not.toHaveBeenCalled()
    })

    it('closes the drawer, not a layer opened below it', () => {
        const closeDrawer = vi.fn()
        const closeSheet = vi.fn()
        const below = (drawerOpen: boolean) => (
            <Shell>
                <Sheet isOpen onClose={closeSheet} testID="below-sheet">
                    <Text>below sheet</Text>
                </Sheet>
                <DrawerAround isOpen={drawerOpen} onClose={closeDrawer} />
            </Shell>
        )
        // The sheet opens first; the drawer opens above it in a later commit.
        const { rerender } = render(below(false))
        rerender(below(true))

        pressEscape()
        expect(closeDrawer).toHaveBeenCalledTimes(1)
        expect(closeSheet).not.toHaveBeenCalled()
    })
})

// A press as a browser delivers it: a pointerdown, then the click of the same
// pointer on the element under it. The stub renders testID as a plain attribute.
function pressBackdrop() {
    const backdrop = document.querySelector<HTMLElement>('[testid="drawer-backdrop"]')
    if (!backdrop) throw new Error('drawer backdrop not rendered')
    act(() => {
        fireEvent.pointerDown(backdrop, { pointerId: 1 })
    })
    act(() => {
        backdrop.dispatchEvent(new PointerEvent('click', { pointerId: 1, bubbles: true }))
    })
}

describe('A press on the backdrop of a Drawer', () => {
    afterEach(() => {
        cleanup()
        resetLayers()
    })

    it('closes only a popover menu open in the drawer, then the drawer on the next press', () => {
        const closeDrawer = vi.fn()
        const menu = (isOpen: boolean) => (
            <Shell>
                <DrawerAround onClose={closeDrawer}>
                    <Menu
                        isOpen={isOpen}
                        onOpenChange={open => rerender(menu(open))}
                        anchor={{ x: 10, y: 10 }}
                        presentation="popover"
                    >
                        <Menu.Item label="North" onSelect={() => {}} />
                    </Menu>
                </DrawerAround>
            </Shell>
        )
        const { rerender, queryByText } = render(menu(false))
        rerender(menu(true))
        expect(queryByText('North')).not.toBeNull()

        pressBackdrop()
        expect(queryByText('North')).toBeNull()
        expect(closeDrawer).not.toHaveBeenCalled()

        pressBackdrop()
        expect(closeDrawer).toHaveBeenCalledTimes(1)
    })

    it('closes the drawer by default', () => {
        const closeDrawer = vi.fn()
        render(
            <Shell>
                <DrawerAround onClose={closeDrawer} />
            </Shell>
        )

        pressBackdrop()
        expect(closeDrawer).toHaveBeenCalledTimes(1)
    })

    it('keeps the drawer open when closeOnOverlayClick is false', () => {
        const closeDrawer = vi.fn()
        render(
            <Shell>
                <DrawerAround onClose={closeDrawer} closeOnOverlayClick={false} />
            </Shell>
        )

        pressBackdrop()
        expect(closeDrawer).not.toHaveBeenCalled()
    })
})

// The react-native stub's BackHandler (tests/react-native-stub.cjs) calls the
// listeners newest first, as React Native does, and has this seam to press
// the button.
type StubBackHandler = typeof BackHandler & { fireHardwareBackPress: () => boolean }

function pressBack(): boolean {
    let handled = false
    act(() => {
        handled = (BackHandler as StubBackHandler).fireHardwareBackPress()
    })
    return handled
}

describe('Android back in a layer inside a Drawer', () => {
    const originalOS = Platform.OS

    beforeEach(() => {
        Platform.OS = 'android'
    })

    afterEach(() => {
        cleanup()
        resetLayers()
        Platform.OS = originalOS
    })

    it('closes only a sheet opened inside the drawer after the drawer re-renders with a new onClose, then the drawer', () => {
        const closeDrawer = vi.fn()
        const closeSheet = vi.fn()
        // A new onClose identity on every render, as a screen's inline handler.
        const sheet = (isOpen: boolean) => (
            <Shell>
                <DrawerAround onClose={() => closeDrawer()}>
                    <Sheet isOpen={isOpen} onClose={closeSheet} testID="drawer-sheet">
                        <Text>drawer sheet</Text>
                    </Sheet>
                </DrawerAround>
            </Shell>
        )
        const { rerender } = render(sheet(false))
        rerender(sheet(true))
        // The drawer re-renders after the sheet opened.
        rerender(sheet(true))

        expect(pressBack()).toBe(true)
        expect(closeSheet).toHaveBeenCalledTimes(1)
        expect(closeDrawer).not.toHaveBeenCalled()

        rerender(sheet(false))
        expect(pressBack()).toBe(true)
        expect(closeDrawer).toHaveBeenCalledTimes(1)
    })
})
