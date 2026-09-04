// @vitest-environment happy-dom

import { cleanup, render } from '@testing-library/react'
import { Menu } from '@tinycld/core/ui/menu'
import { Text } from 'react-native'
import { afterEach, describe, expect, it } from 'vitest'

/**
 * Menu.Overlay must render NOTHING on web.
 *
 * It used to be a full-screen absolutely-positioned Pressable, and that made
 * every submenu item unclickable: Overlay is a SIBLING of Menu.Content inside
 * the Portal, while SubContent is a CHILD of Content positioned outside
 * Content's box — so the overlay won hit-testing wherever the submenu drew.
 * SubContent's own `zIndex: 50` could not help, applying as it does only
 * within Content's stacking context.
 *
 * Playwright reported it as "<div … absolute top-0 left-0 right-0 bottom-0>
 * intercepts pointer events", retried 40 times, until the test timed out. A
 * mouse user saw an open submenu whose items did nothing.
 *
 * Web dismissal now lives in a document-level listener in Menu.Portal, which
 * gets submenus right because SubContent genuinely IS a DOM descendant of
 * Content. This file guards the half of that fix a unit test can reach: the
 * Portal itself does not mount under the react-native stub (gluestack's
 * Overlay has no DOM path there), which is exactly why the interaction went
 * untested and shipped. The end-to-end proof is cards'
 * tests/e2e/list-status.spec.ts, which clicks a submenu item for real.
 */
describe('Menu.Overlay on web', () => {
    afterEach(cleanup)

    it('renders no dismiss layer', () => {
        const { container } = render(
            <Menu isOpen>
                <Menu.Overlay />
                <Menu.Item onPress={() => {}} testID="item">
                    <Text>Pick me</Text>
                </Menu.Item>
            </Menu>
        )

        // The overlay's signature: a full-bleed absolutely-positioned box.
        // Matching on the class rather than a testID because that is what the
        // browser hit-tests, and what CI named when it failed.
        const fullBleed = container.querySelectorAll(
            '[class*="absolute"][class*="top-0"][class*="left-0"][class*="right-0"][class*="bottom-0"]'
        )
        expect(fullBleed).toHaveLength(0)
    })

    it('leaves the rest of the menu alone', () => {
        const { queryByTestId } = render(
            <Menu isOpen>
                <Menu.Overlay />
                <Menu.Item onPress={() => {}} testID="item">
                    <Text>Pick me</Text>
                </Menu.Item>
            </Menu>
        )

        // Rendering nothing for the overlay must not take its siblings with
        // it — the 38 call sites that render <Menu.Overlay /> keep working.
        expect(queryByTestId('item')).not.toBeNull()
    })
})
