// @vitest-environment happy-dom

import { cleanup, fireEvent, render } from '@testing-library/react'
import { Menu } from '@tinycld/core/ui/menu'
import { Text } from 'react-native'
import { afterEach, describe, expect, it } from 'vitest'

/**
 * On web a submenu is hoisted out of Content's ScrollView (which clips and
 * traps it) into a slot beside it. The slot is discovered through a ref, and
 * that hand-off is where a render loop once lived: an inline ref callback is
 * re-attached on every render, and publishing the node from it re-rendered the
 * menu, which re-attached the ref, and so on until React gave up — every
 * plain menu in the app remounted its items on open, and Playwright reported
 * each one as "element was detached from the DOM".
 *
 * Rendered without <Menu.Portal>, which does not mount under the react-native
 * stub; Content itself carries the slot.
 */
describe('Menu submenu slot (web)', () => {
    afterEach(cleanup)

    it('mounts Content without looping', () => {
        const { getByTestId } = render(
            <Menu isOpen>
                <Menu.Content>
                    <Menu.Item onPress={() => {}} testID="item">
                        <Text>Pick me</Text>
                    </Menu.Item>
                </Menu.Content>
            </Menu>
        )
        expect(getByTestId('item')).not.toBeNull()
    })

    it('renders an open submenu inside Content but outside its ScrollView', () => {
        const { getByTestId, getByText } = render(
            <Menu isOpen>
                <Menu.Content>
                    <Menu.Sub>
                        <Menu.SubTrigger>
                            <Text>More</Text>
                        </Menu.SubTrigger>
                        <Menu.SubContent>
                            <Menu.Item onPress={() => {}} testID="sub-item">
                                <Text>Nested</Text>
                            </Menu.Item>
                        </Menu.SubContent>
                    </Menu.Sub>
                </Menu.Content>
            </Menu>
        )

        fireEvent.click(getByText('More'))

        const subItem = getByTestId('sub-item')
        const scroll = subItem.ownerDocument.querySelector('rn-scrollview')
        expect(scroll).not.toBeNull()
        expect(scroll?.contains(subItem)).toBe(false)
        // Still a DOM descendant of Content (the ScrollView's parent), so the
        // outside-click listener reads a submenu click as inside the menu.
        expect(scroll?.parentElement?.contains(subItem)).toBe(true)
    })
})
