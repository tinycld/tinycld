// @vitest-environment happy-dom

import { OverlayProvider as PortalHost } from '@gluestack-ui/core/overlay/creator'
import { cleanup, render } from '@testing-library/react'
import { Drawer, DrawerBackdrop, DrawerBody, DrawerContent } from '@tinycld/core/ui/drawer'
import { OverlayHost, OverlayProvider } from '@tinycld/core/ui/overlay'
import { resetLayers } from '@tinycld/core/ui/overlay/layer-stack'
import { Sheet } from '@tinycld/core/ui/sheet'
import type { ReactNode } from 'react'
import { Text, View } from 'react-native'
import { SafeAreaProvider } from 'react-native-safe-area-context'
import { afterEach, describe, expect, it } from 'vitest'

/**
 * The mobile shape: the `sheet` host nested inside the app content (as
 * MobileLayout mounts it), the root host from OverlayProvider, and gluestack's
 * portal host, which an overlay Drawer renders through. That portal draws
 * after the app content, so a Sheet in the `sheet` host is covered by any
 * open overlay Drawer: a Sheet opened from inside a Drawer must render in the
 * root host instead.
 */
function MobileShell({ children }: { children: ReactNode }) {
    return (
        <OverlayProvider>
            <PortalHost>
                <div data-testid="mobile-content">
                    <OverlayHost name="sheet" />
                </div>
                {children}
            </PortalHost>
        </OverlayProvider>
    )
}

function surfaceOf(testID: string) {
    const surface = document.querySelector<HTMLElement>(`[testid="${testID}"]`)
    if (!surface) throw new Error(`sheet surface ${testID} not rendered`)
    return surface
}

const sheetRegion = () => {
    const region = document.querySelector<HTMLElement>('[data-testid="mobile-content"]')
    if (!region) throw new Error('sheet host region not rendered')
    return region
}

describe('Sheet inside a Drawer', () => {
    afterEach(() => {
        cleanup()
        resetLayers()
    })

    it('renders a sheet opened inside an overlay drawer in the root host, not the sheet host', () => {
        render(
            <MobileShell>
                <Sheet isOpen onClose={() => {}} testID="page-sheet">
                    <Text>page sheet</Text>
                </Sheet>
                <Drawer isOpen onClose={() => {}} anchor="right">
                    <DrawerBackdrop />
                    <DrawerContent>
                        <DrawerBody>
                            <Text>drawer body</Text>
                            <Sheet isOpen onClose={() => {}} side="top" testID="drawer-sheet">
                                <Text>drawer sheet</Text>
                            </Sheet>
                        </DrawerBody>
                    </DrawerContent>
                </Drawer>
            </MobileShell>
        )

        // A sheet of the page itself still rests in the mobile content region.
        expect(sheetRegion().contains(surfaceOf('page-sheet'))).toBe(true)
        // The drawer's sheet must not: the drawer's gluestack portal paints
        // over that region and would cover it.
        expect(sheetRegion().contains(surfaceOf('drawer-sheet'))).toBe(false)
    })

    it('renders a sheet that is a sibling of the drawer content in the root host', () => {
        render(
            <MobileShell>
                <Drawer isOpen onClose={() => {}} anchor="right">
                    <DrawerBackdrop />
                    <DrawerContent>
                        <DrawerBody>
                            <Text>drawer body</Text>
                        </DrawerBody>
                    </DrawerContent>
                    <Sheet isOpen onClose={() => {}} testID="sibling-sheet">
                        <Text>sibling sheet</Text>
                    </Sheet>
                </Drawer>
            </MobileShell>
        )

        // The sheet is inside the drawer's portal too, so the drawer paints
        // over the `sheet` host region and would cover it there.
        expect(sheetRegion().contains(surfaceOf('sibling-sheet'))).toBe(false)
    })
})

/**
 * Device insets of a notched phone. Web e2e always reads insets of 0, so the
 * safe-area padding of a sheet that takes the root host is checked only here.
 */
const PHONE_INSETS = { top: 47, right: 0, bottom: 34, left: 0 }
const PHONE_METRICS = {
    insets: PHONE_INSETS,
    frame: { x: 0, y: 0, width: 390, height: 844 },
}

function NotchedShell({ children }: { children: ReactNode }) {
    return (
        <SafeAreaProvider initialMetrics={PHONE_METRICS}>
            <MobileShell>{children}</MobileShell>
        </SafeAreaProvider>
    )
}

function DrawerWith({ children }: { children: ReactNode }) {
    return (
        <Drawer isOpen onClose={() => {}} anchor="right">
            <DrawerBackdrop />
            <DrawerContent>
                <DrawerBody>{children}</DrawerBody>
            </DrawerContent>
        </Drawer>
    )
}

/** A style length in px; an unset padding is 0. */
function px(value: string) {
    return value === '' ? 0 : Number.parseFloat(value)
}

/** The Sheet.Footer row: the parent of the content a test puts in it. */
function footerOf(contentTestID: string) {
    const footer = surfaceOf(contentTestID).parentElement
    if (!footer) throw new Error(`footer of ${contentTestID} not rendered`)
    return footer
}

describe('Sheet inside a Drawer: safe-area insets of the root host', () => {
    afterEach(() => {
        cleanup()
        resetLayers()
    })

    it('pads a top sheet inside a drawer by the top inset', () => {
        render(
            <NotchedShell>
                <DrawerWith>
                    <Sheet isOpen onClose={() => {}} side="top" testID="top-sheet">
                        <Text>top sheet</Text>
                    </Sheet>
                </DrawerWith>
            </NotchedShell>
        )

        expect(px(surfaceOf('top-sheet').style.paddingTop)).toBe(PHONE_INSETS.top)
    })

    it('pads a bottom sheet with no footer inside a drawer by the bottom inset', () => {
        render(
            <NotchedShell>
                <DrawerWith>
                    <Sheet isOpen onClose={() => {}} testID="bottom-sheet">
                        <Text>bottom sheet</Text>
                    </Sheet>
                </DrawerWith>
            </NotchedShell>
        )

        // The root host reaches the screen edge, so the last row must clear
        // the home indicator.
        expect(px(surfaceOf('bottom-sheet').style.paddingBottom)).toBe(PHONE_INSETS.bottom)
    })

    it('adds the bottom inset once to a bottom sheet with a footer inside a drawer', () => {
        render(
            <NotchedShell>
                <DrawerWith>
                    <Sheet isOpen onClose={() => {}} testID="footer-sheet">
                        <Sheet.Footer>
                            <View testID="footer-content" />
                        </Sheet.Footer>
                    </Sheet>
                </DrawerWith>
            </NotchedShell>
        )

        // The surface clears the home indicator, so the footer keeps only its
        // own 12 px and does not add the inset a second time.
        expect(px(surfaceOf('footer-sheet').style.paddingBottom)).toBe(PHONE_INSETS.bottom)
        expect(px(footerOf('footer-content').style.paddingBottom)).toBe(12)
    })

    it('adds no surface bottom inset to a bottom sheet outside a drawer', () => {
        render(
            <NotchedShell>
                <Sheet isOpen onClose={() => {}} testID="page-sheet">
                    <Text>page sheet</Text>
                </Sheet>
            </NotchedShell>
        )

        // The sheet host already ends above the home indicator (at the tab bar).
        expect(sheetRegion().contains(surfaceOf('page-sheet'))).toBe(true)
        expect(px(surfaceOf('page-sheet').style.paddingBottom)).toBe(0)
    })
})
