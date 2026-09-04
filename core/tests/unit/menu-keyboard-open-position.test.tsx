// @vitest-environment happy-dom

import { cleanup, render } from '@testing-library/react'
import { Menu } from '@tinycld/core/ui/menu'
import { Pressable, Text } from 'react-native'
import { afterEach, describe, expect, it, vi } from 'vitest'

/**
 * A menu opened without a pointer must still land beside its trigger.
 *
 * Trigger used to measure its rect only from its own click and hover
 * handlers, so a menu flipped open by a keyboard shortcut (or by a parent
 * setting a controlled `isOpen`) had no rect: Content positioned to `{}` and
 * drew at the container origin. Cards deferred its d/l/a/p/f shortcuts on
 * exactly this.
 *
 * Rendered without <Menu.Portal>, which does not mount under the react-native
 * stub. Content reads the same context either way.
 */
describe('Menu opened without a pointer (web)', () => {
    afterEach(() => {
        cleanup()
        vi.restoreAllMocks()
    })

    it('positions Content from the trigger rect', () => {
        const rect = { left: 40, top: 20, width: 100, height: 30 }
        vi.spyOn(Element.prototype, 'getBoundingClientRect').mockReturnValue(rect as DOMRect)

        const { getByTestId } = render(
            <Menu isOpen>
                <Menu.Trigger>
                    <Pressable testID="trigger">
                        <Text>Open</Text>
                    </Pressable>
                </Menu.Trigger>
                <Menu.Content>
                    <Menu.Item onPress={() => {}} testID="item">
                        <Text>Pick me</Text>
                    </Menu.Item>
                </Menu.Content>
            </Menu>
        )

        const content = getByTestId('item').closest('rn-scrollview')?.parentElement as HTMLElement
        expect(content.style.left).toBe('40px')
        // Below the trigger, plus the 4px gap Content always leaves.
        expect(content.style.top).toBe('54px')
    })
})
