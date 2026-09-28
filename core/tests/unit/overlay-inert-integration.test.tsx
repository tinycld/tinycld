// @vitest-environment happy-dom

import { render } from '@testing-library/react'
import { OverlayHost, OverlayPortal, OverlayProvider } from '@tinycld/core/ui/overlay'
import { resetLayers, useOverlayLayer } from '@tinycld/core/ui/overlay/layer-stack'
// Directly, not through the barrel — the same path the real consumers use, so
// this test exercises the import shape that ships.
import { useInertExempt } from '@tinycld/core/ui/overlay/use-inert-exempt'
import { Text, View } from 'react-native'
import { afterEach, describe, expect, it } from 'vitest'

/**
 * The inert boundary must contain the app subtree ONLY.
 *
 * Everything that is meant to sit ON TOP of a modal has to stay interactive
 * while it is open: every overlay host (including the `sheet` host, which is
 * deliberately nested deep inside the app content so a bottom sheet rests on
 * the mobile tab bar), and every always-on-top surface that renders in place
 * rather than through a host — the toast renderer and the offline overlay.
 *
 * A toast raised by a dialog's own save is the case that motivated this: it
 * had a dead Dismiss and Undo and was invisible to a screen reader, at exactly
 * the moment it had something to say.
 */

afterEach(() => {
    resetLayers()
})

// react-native-web renders testID as a lowercase `testid` attribute, which
// Testing Library's getByTestId does not look for.
function byTestId(container: HTMLElement, id: string): Element {
    const node = container.querySelector(`[testid="${id}"]`)
    if (!node) throw new Error(`no element with testid="${id}"`)
    return node
}

function hasInertAncestor(node: Element | null): boolean {
    let cur: Element | null = node
    while (cur) {
        if (cur.hasAttribute?.('inert')) return true
        cur = cur.parentElement
    }
    return false
}

// A modal layer that portals into the named host, exactly as Dialog and Sheet
// do. Portalling is the point: a surface rendered in place IS app content and
// should be inert — only what lands in a host is on top of the modal.
function ModalLayer({ host, testID }: { host: 'root' | 'sheet'; testID: string }) {
    useOverlayLayer({
        isOpen: true,
        nodes: () => [],
        onDismiss: () => {},
        dismissOnOutside: false,
        isModal: true,
    })
    return (
        <OverlayPortal host={host}>
            <View testID={testID}>
                <Text>surface</Text>
            </View>
        </OverlayPortal>
    )
}

// Stands in for ToastRenderer / OfflineOverlay: renders in place, exempt.
function InPlaceSurface({ testID }: { testID: string }) {
    const ref = useInertExempt()
    return (
        <View ref={ref} testID={testID}>
            <Text>toast</Text>
        </View>
    )
}

describe('the inert boundary', () => {
    it('holds the app out while keeping every host and in-place surface live', () => {
        const { container } = render(
            <OverlayProvider>
                {/* The mobile shape: app chrome, with the sheet host nested
                    inside the content region where a Sheet needs it. */}
                <View testID="app-root">
                    <View testID="package-tabs">
                        <Text>app content</Text>
                    </View>
                    <OverlayHost name="sheet" />
                </View>
                <InPlaceSurface testID="toast-stack" />
                {/* A modal in each host: a dialog opened over a sheet. */}
                <ModalLayer host="sheet" testID="sheet-modal" />
                <ModalLayer host="root" testID="root-modal" />
            </OverlayProvider>
        )

        // The app content is held out...
        expect(hasInertAncestor(byTestId(container, 'package-tabs'))).toBe(true)

        // ...while nothing meant to sit on top of it is.
        expect(hasInertAncestor(byTestId(container, 'toast-stack'))).toBe(false)
        expect(hasInertAncestor(byTestId(container, 'sheet-modal'))).toBe(false)
        expect(hasInertAncestor(byTestId(container, 'root-modal'))).toBe(false)
    })

    it('leaves the app interactive when no modal is open', () => {
        const { container } = render(
            <OverlayProvider>
                <View testID="app-root">
                    <View testID="package-tabs">
                        <Text>app content</Text>
                    </View>
                    <OverlayHost name="sheet" />
                </View>
                <InPlaceSurface testID="toast-stack" />
            </OverlayProvider>
        )

        expect(hasInertAncestor(byTestId(container, 'package-tabs'))).toBe(false)
        expect(hasInertAncestor(byTestId(container, 'toast-stack'))).toBe(false)
    })

    // A menu or popover is not modal: the page behind it stays usable.
    it('leaves the app interactive for a non-modal layer', () => {
        function Menu() {
            useOverlayLayer({ isOpen: true, nodes: () => [], onDismiss: () => {} })
            return null
        }
        const { container } = render(
            <OverlayProvider>
                <View testID="app-root">
                    <View testID="package-tabs">
                        <Text>app content</Text>
                    </View>
                </View>
                <Menu />
            </OverlayProvider>
        )

        expect(hasInertAncestor(byTestId(container, 'package-tabs'))).toBe(false)
    })
})
