// @vitest-environment happy-dom

import { cleanup, fireEvent, render } from '@testing-library/react'
import { Menu } from '@tinycld/core/ui/menu'
import { Text } from 'react-native'
import { afterEach, describe, expect, it, vi } from 'vitest'

/**
 * A menu item has to be operable without a mouse.
 *
 * RN's Pressable renders a bare div on web — no tabIndex, no key handling — so
 * a menu built from ordinary items was pointer-only: invisible to Tab and
 * unactivatable by Enter. The href variant gets this free from <a> and
 * SubTrigger hand-rolls it, which left plain items as the gap.
 */
describe('Menu.Item keyboard reachability (web)', () => {
    afterEach(cleanup)

    // Rendered directly rather than through <Menu.Portal>: the portal goes
    // through gluestack's Overlay, which does not mount under the react-native
    // stub, so nothing inside it exists in the DOM to assert against. The item
    // only needs the surrounding context, which <Menu> alone provides.
    function renderItem(onPress: () => void, isDisabled = false) {
        return render(
            <Menu isOpen>
                <Menu.Item onPress={onPress} isDisabled={isDisabled} testID="item">
                    <Text>Pick me</Text>
                </Menu.Item>
            </Menu>
        )
    }

    it('puts the item in the tab order', () => {
        const { getByTestId } = renderItem(() => {})
        expect(getByTestId('item').getAttribute('tabindex')).toBe('0')
    })

    it('activates on Enter', () => {
        const onPress = vi.fn()
        const { getByTestId } = renderItem(onPress)

        fireEvent.keyDown(getByTestId('item'), { key: 'Enter' })

        expect(onPress).toHaveBeenCalledOnce()
    })

    /**
     * Space activates a menuitem per ARIA, and without preventDefault it would
     * scroll the popover instead of choosing the row under the caret.
     */
    it('activates on Space', () => {
        const onPress = vi.fn()
        const { getByTestId } = renderItem(onPress)

        fireEvent.keyDown(getByTestId('item'), { key: ' ' })

        expect(onPress).toHaveBeenCalledOnce()
    })

    it('ignores keys that are not activation keys', () => {
        const onPress = vi.fn()
        const { getByTestId } = renderItem(onPress)

        fireEvent.keyDown(getByTestId('item'), { key: 'a' })

        expect(onPress).not.toHaveBeenCalled()
    })

    it('drops a disabled item out of the tab order and does not activate it', () => {
        const onPress = vi.fn()
        const { getByTestId } = renderItem(onPress, true)

        const item = getByTestId('item')
        expect(item.getAttribute('tabindex')).toBe('-1')

        fireEvent.keyDown(item, { key: 'Enter' })
        expect(onPress).not.toHaveBeenCalled()
    })

    it('exposes the menuitem role', () => {
        const { getByTestId } = renderItem(() => {})
        expect(getByTestId('item').getAttribute('role')).toBe('menuitem')
    })
})
