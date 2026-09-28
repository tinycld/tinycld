// @vitest-environment happy-dom

import { applyInertSiblings, clearInertSiblings } from '@tinycld/core/ui/overlay/inert-siblings'
import { afterEach, describe, expect, it } from 'vitest'

/**
 * A modal layer must hold the rest of the document out of play — but never the
 * layer itself. That distinction is the whole reason this is not one `inert`
 * wrapper around the app: `inert` cascades to the entire subtree and a
 * descendant cannot undo it, and the `sheet` host is deliberately nested deep
 * inside the app content so a bottom sheet rests ON the mobile tab bar rather
 * than under it. Wrapping the app made every bottom sheet inert the moment it
 * opened, which is the regression these tests pin.
 */

function el(tag = 'div'): HTMLElement {
    return document.createElement(tag)
}

function isInert(node: HTMLElement): boolean {
    return node.hasAttribute('inert')
}

afterEach(() => {
    clearInertSiblings()
    document.body.innerHTML = ''
})

/**
 * The mobile shape, which is the one that regressed:
 *
 *   body
 *     └ appRoot
 *         ├ banner            ← app chrome, must go inert
 *         ├ contentBox
 *         │   ├ packageTabs   ← app chrome, must go inert
 *         │   └ sheetHost     ← a HOST, nested two deep, must stay live
 *         └ tabBar            ← app chrome, must go inert
 *     └ rootHost              ← a HOST, must stay live
 */
function mountMobileTree() {
    const appRoot = el()
    const banner = el()
    const contentBox = el()
    const packageTabs = el()
    const sheetHost = el()
    const sheetContent = el()
    const tabBar = el()
    const rootHost = el()

    sheetHost.appendChild(sheetContent)
    contentBox.append(packageTabs, sheetHost)
    appRoot.append(banner, contentBox, tabBar)
    document.body.append(appRoot, rootHost)

    return { appRoot, banner, contentBox, packageTabs, sheetHost, sheetContent, tabBar, rootHost }
}

describe('applyInertSiblings', () => {
    it('keeps a deeply nested sheet host interactive while the app goes inert', () => {
        const t = mountMobileTree()
        applyInertSiblings([t.rootHost, t.sheetHost])

        // The hosts and every ancestor on their path stay live.
        expect(isInert(t.sheetHost)).toBe(false)
        expect(isInert(t.sheetContent)).toBe(false)
        expect(isInert(t.rootHost)).toBe(false)
        expect(isInert(t.contentBox)).toBe(false)
        expect(isInert(t.appRoot)).toBe(false)

        // Everything off those paths is held out.
        expect(isInert(t.banner)).toBe(true)
        expect(isInert(t.packageTabs)).toBe(true)
        expect(isInert(t.tabBar)).toBe(true)
    })

    // A dialog in the "root" host opened OVER a sheet in the "sheet" host:
    // both hosts are live, so both surfaces stay usable and the app behind
    // them does not.
    it('keeps both hosts interactive with a modal in each', () => {
        const t = mountMobileTree()
        applyInertSiblings([t.rootHost, t.sheetHost])

        expect(isInert(t.rootHost)).toBe(false)
        expect(isInert(t.sheetHost)).toBe(false)
        expect(isInert(t.packageTabs)).toBe(true)
    })

    it('holds the app out when only the root host exists (desktop)', () => {
        const t = mountMobileTree()
        t.sheetHost.remove()
        applyInertSiblings([t.rootHost, undefined])

        expect(isInert(t.rootHost)).toBe(false)
        expect(isInert(t.appRoot)).toBe(true)
    })

    it('clears every mark it set', () => {
        const t = mountMobileTree()
        applyInertSiblings([t.rootHost, t.sheetHost])
        clearInertSiblings()

        for (const node of Object.values(t)) {
            expect(isInert(node)).toBe(false)
        }
    })

    // The tree moves under this — a host mounts, a screen re-renders — so each
    // call recomputes from scratch. A stale mark would strand an `inert` on
    // something the user needs.
    it('recomputes rather than accumulating across calls', () => {
        const t = mountMobileTree()
        applyInertSiblings([t.rootHost, t.sheetHost])
        expect(isInert(t.packageTabs)).toBe(true)

        // The sheet closed: its host is gone, so its former siblings are now
        // just app chrome, and the box that held it must go inert too.
        t.sheetHost.remove()
        applyInertSiblings([t.rootHost, undefined])
        expect(isInert(t.appRoot)).toBe(true)
        expect(isInert(t.packageTabs)).toBe(false) // inert via its ancestor now
    })

    // Inerting the whole document with no host would lock the user out of the
    // page with nothing on top of it to use instead.
    it('does nothing when no host is live', () => {
        const t = mountMobileTree()
        applyInertSiblings([])
        expect(isInert(t.appRoot)).toBe(false)
    })

    it('ignores a host that is not in the document', () => {
        const t = mountMobileTree()
        applyInertSiblings([el()])
        expect(isInert(t.appRoot)).toBe(false)
    })

    // Something inert for its own reasons must come back inert, not be
    // cleared by this module on the next pass.
    it('leaves an inert it did not set alone', () => {
        const t = mountMobileTree()
        t.banner.setAttribute('inert', '')
        applyInertSiblings([t.rootHost, t.sheetHost])
        clearInertSiblings()
        expect(isInert(t.banner)).toBe(true)
    })
})
