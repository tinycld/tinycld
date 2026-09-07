// @vitest-environment happy-dom

import { cleanup, fireEvent, render as renderBare } from '@testing-library/react'
import { Menu } from '@tinycld/core/ui/menu'
import { OverlayProvider } from '@tinycld/core/ui/overlay'
import type { ReactElement } from 'react'
import { Pressable, Text } from 'react-native'
import { afterEach, describe, expect, it, vi } from 'vitest'

// Every surface renders through the overlay host, which the app mounts once.
const render = (ui: ReactElement) => renderBare(<OverlayProvider>{ui}</OverlayProvider>)

const rows = (root: ParentNode) =>
    Array.from(root.querySelectorAll<HTMLElement>('[role="menuitem"]'))

describe('Menu (web)', () => {
    afterEach(cleanup)

    it('renders nothing until opened, then rows with menu roles inside the host', () => {
        const onSelect = vi.fn()
        const { container, queryByText, getByText } = render(
            <Menu
                trigger={
                    <Pressable testID="trigger">
                        <Text>Open</Text>
                    </Pressable>
                }
                presentation="popover"
            >
                <Menu.Item label="Rename" onSelect={onSelect} testID="rename" />
                <Menu.Item label="Delete" isDestructive onSelect={() => {}} />
            </Menu>
        )
        expect(queryByText('Rename')).toBeNull()

        fireEvent.click(getByText('Open'))

        const surface = container.querySelector('[role="menu"]')
        expect(surface).not.toBeNull()
        expect(rows(container).map(row => row.textContent)).toEqual(['Rename', 'Delete'])

        fireEvent.click(getByText('Rename'))
        expect(onSelect).toHaveBeenCalledTimes(1)
        // Choosing a row closes the menu.
        expect(queryByText('Rename')).toBeNull()
    })

    it('activates a focused row on Enter and Space, never a disabled one', () => {
        const onSelect = vi.fn()
        const onDisabled = vi.fn()
        const { getByTestId } = render(
            <Menu isOpen onOpenChange={() => {}} anchor={{ x: 10, y: 10 }} presentation="popover">
                <Menu.Item label="Pick me" onSelect={onSelect} testID="pick" />
                <Menu.Item label="Not me" onSelect={onDisabled} isDisabled testID="no" />
            </Menu>
        )
        fireEvent.keyDown(getByTestId('pick'), { key: 'Enter' })
        fireEvent.keyDown(getByTestId('pick'), { key: ' ' })
        fireEvent.keyDown(getByTestId('pick'), { key: 'a' })
        expect(onSelect).toHaveBeenCalledTimes(2)

        fireEvent.click(getByTestId('no'))
        fireEvent.keyDown(getByTestId('no'), { key: 'Enter' })
        expect(onDisabled).not.toHaveBeenCalled()
        expect(getByTestId('no').getAttribute('aria-disabled')).toBe('true')
    })

    it('moves focus with the arrow keys, wrapping, and skips disabled rows', () => {
        const { container } = render(
            <Menu isOpen onOpenChange={() => {}} anchor={{ x: 10, y: 10 }} presentation="popover">
                <Menu.Item label="One" onSelect={() => {}} testID="one" />
                <Menu.Item label="Two" onSelect={() => {}} isDisabled testID="two" />
                <Menu.Item label="Three" onSelect={() => {}} testID="three" />
            </Menu>
        )
        const surface = container.querySelector<HTMLElement>('[role="menu"]')
        if (!surface) throw new Error('no surface')
        const focused = () => document.activeElement?.getAttribute('data-testid')

        // Opening puts focus on the first row, so the keyboard has somewhere to start.
        expect(focused()).toBe('one')
        fireEvent.keyDown(surface, { key: 'ArrowDown' })
        expect(focused()).toBe('three')
        fireEvent.keyDown(surface, { key: 'ArrowDown' })
        expect(focused()).toBe('one')
        fireEvent.keyDown(surface, { key: 'ArrowUp' })
        expect(focused()).toBe('three')
        fireEvent.keyDown(surface, { key: 'Home' })
        expect(focused()).toBe('one')
        fireEvent.keyDown(surface, { key: 't' })
        expect(focused()).toBe('three')
    })

    it('opens a submenu beside the surface, outside the scroll region, pinned to its origin', () => {
        const { container, getByText, getByTestId } = render(
            <Menu isOpen onOpenChange={() => {}} anchor={{ x: 10, y: 10 }} presentation="popover">
                <Menu.Sub label="Status">
                    <Menu.Item label="Backlog" onSelect={() => {}} testID="sub-item" />
                </Menu.Sub>
            </Menu>
        )
        fireEvent.click(getByText('Status'))

        const subItem = getByTestId('sub-item')
        const scroll = container.querySelector('rn-scrollview')
        expect(scroll).not.toBeNull()
        // Hoisted out of the scroll region (which clips and traps it) but
        // still a DOM descendant of the surface, so an outside-press check
        // reads a submenu click as inside.
        expect(scroll?.contains(subItem)).toBe(false)
        const surface = container.querySelector('[role="menu"]')
        expect(surface?.contains(subItem)).toBe(true)
        // The slot the panel portals into sits at the surface's origin;
        // that is what makes the submenu's offsets land on its trigger row.
        const slot = Array.from(scroll?.parentElement?.children ?? []).find(child =>
            child.contains(subItem)
        ) as HTMLElement
        expect(slot.style.position).toBe('absolute')
        expect(slot.style.top).toBe('0px')
        expect(slot.style.left).toBe('0px')
    })

    it('closes on a press outside the surface and its trigger, and toggles on the trigger', () => {
        const onOpenChange = vi.fn()
        const { getByText, queryByText } = render(
            <Menu
                trigger={
                    <Pressable>
                        <Text>Open</Text>
                    </Pressable>
                }
                onOpenChange={onOpenChange}
                presentation="popover"
            >
                <Menu.Item label="Row" onSelect={() => {}} />
            </Menu>
        )
        fireEvent.click(getByText('Open'))
        expect(queryByText('Row')).not.toBeNull()

        fireEvent.pointerDown(document.body)
        expect(queryByText('Row')).toBeNull()
        expect(onOpenChange).toHaveBeenLastCalledWith(false)

        fireEvent.click(getByText('Open'))
        expect(queryByText('Row')).not.toBeNull()
        fireEvent.click(getByText('Open'))
        expect(queryByText('Row')).toBeNull()
    })

    it('closes on Escape even when a field inside it has focus', () => {
        const onOpenChange = vi.fn()
        const { getByPlaceholderText } = render(
            <Menu
                isOpen
                onOpenChange={onOpenChange}
                anchor={{ x: 10, y: 10 }}
                presentation="popover"
            >
                <Menu.Custom>
                    <input placeholder="Search" />
                </Menu.Custom>
                <Menu.Item label="Row" onSelect={() => {}} />
            </Menu>
        )
        const field = getByPlaceholderText('Search')
        field.focus()
        fireEvent.keyDown(field, { key: 'Escape' })
        expect(onOpenChange).toHaveBeenLastCalledWith(false)
    })

    it('renders a checkbox row that toggles without closing', () => {
        const onToggle = vi.fn()
        const { getByText, queryByText } = render(
            <Menu isOpen onOpenChange={() => {}} anchor={{ x: 10, y: 10 }} presentation="popover">
                <Menu.CheckboxItem label="Show done" isChecked onToggle={onToggle} />
            </Menu>
        )
        const row = getByText('Show done').closest('[role="menuitemcheckbox"]')
        expect(row?.getAttribute('aria-checked')).toBe('true')
        fireEvent.click(getByText('Show done'))
        expect(onToggle).toHaveBeenCalledTimes(1)
        expect(queryByText('Show done')).not.toBeNull()
    })
})
