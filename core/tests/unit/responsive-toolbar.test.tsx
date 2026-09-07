// @vitest-environment happy-dom

import { cleanup, fireEvent, render as renderBare } from '@testing-library/react'
import { ResponsiveToolbar, type ToolbarItem } from '@tinycld/core/components/ResponsiveToolbar'
import { Menu } from '@tinycld/core/ui/menu'
import { OverlayProvider } from '@tinycld/core/ui/overlay'
import { Bold, Italic, Underline } from 'lucide-react-native'
import type { ReactElement } from 'react'
import { Text } from 'react-native'
import { afterEach, describe, expect, it, vi } from 'vitest'

const render = (ui: ReactElement) => renderBare(<OverlayProvider>{ui}</OverlayProvider>)

/** Report a box to the stub the way react-native-web's ResizeObserver would. */
const fireLayout = (el: Element | null, width: number) => {
    if (!el) throw new Error('no element to lay out')
    fireEvent(el, new CustomEvent('rn-layout', { detail: { x: 0, y: 0, width, height: 44 } }))
}

const q = (root: ParentNode, testId: string) => root.querySelector(`[testid="${testId}"]`)
const rows = (root: ParentNode) =>
    Array.from(root.querySelectorAll<HTMLElement>('[role="menuitem"]')).map(r => r.textContent)

const button = (key: string, onPress = () => {}): ToolbarItem => ({
    type: 'button',
    key,
    icon: Bold,
    label: key.toUpperCase(),
    onPress,
})

/** Row of 200 with four 40px buttons and a 34px More button. */
function layoutFour(container: HTMLElement, rowWidth = 200) {
    for (const key of ['a', 'b', 'c', 'd']) fireLayout(q(container, `toolbar-item-${key}`), 40)
    fireLayout(q(container, 'toolbar-more'), 34)
    fireLayout(q(container, 'toolbar-row'), rowWidth)
}

describe('ResponsiveToolbar', () => {
    afterEach(cleanup)

    it('renders every item invisible and inert until the row is measured', () => {
        const { container, getByRole } = render(
            <ResponsiveToolbar items={[button('a'), button('b')]} />
        )
        const row = q(container, 'toolbar-row') as HTMLElement
        expect(row.style.opacity).toBe('0')
        expect(row.style.pointerEvents).toBe('none')
        expect(getByRole('button', { name: 'A' })).toBeTruthy()
        expect(getByRole('button', { name: 'B' })).toBeTruthy()
        expect(q(container, 'toolbar-more')).not.toBeNull()
    })

    it('folds what does not fit into the More menu, and the row acts on it', () => {
        const onD = vi.fn()
        const { container, queryByRole, getByRole } = render(
            <ResponsiveToolbar items={[button('a'), button('b'), button('c'), button('d', onD)]} />
        )
        layoutFour(container, 162)

        const row = q(container, 'toolbar-row') as HTMLElement
        expect(row.style.opacity).toBe('1')
        // Four 40px buttons plus gaps need 166; 162 holds three and the More button.
        expect(getByRole('button', { name: 'C' })).toBeTruthy()
        expect(queryByRole('button', { name: 'D' })).toBeNull()

        fireEvent.click(q(container, 'toolbar-more-button') as Element)
        expect(rows(container)).toEqual(['D'])
        fireEvent.click(getByRole('menuitem', { name: 'D' }))
        expect(onD).toHaveBeenCalledTimes(1)
    })

    it('keeps a pinned custom item in the row and out of the menu', () => {
        const items: ToolbarItem[] = [
            { type: 'custom', key: 'title', element: <Text>Title</Text> },
            button('a'),
            button('b'),
        ]
        const { container, getByText, queryByRole } = render(<ResponsiveToolbar items={items} />)
        fireLayout(q(container, 'toolbar-item-title'), 120)
        fireLayout(q(container, 'toolbar-item-a'), 40)
        fireLayout(q(container, 'toolbar-item-b'), 40)
        fireLayout(q(container, 'toolbar-more'), 34)
        fireLayout(q(container, 'toolbar-row'), 160)

        expect(getByText('Title')).toBeTruthy()
        expect(queryByRole('button', { name: 'A' })).toBeNull()
        fireEvent.click(q(container, 'toolbar-more-button') as Element)
        expect(rows(container)).toEqual(['A', 'B'])
    })

    it('always shows More with a permanent menu, overflow above its rows', () => {
        const items = [button('a'), button('b')]
        const menu = <Menu.Item label="Archive" onSelect={() => {}} />
        const { container } = renderBare(
            <OverlayProvider>
                <ResponsiveToolbar items={items} moreMenu={menu} />
            </OverlayProvider>
        )
        fireLayout(q(container, 'toolbar-item-a'), 40)
        fireLayout(q(container, 'toolbar-item-b'), 40)
        fireLayout(q(container, 'toolbar-more'), 34)
        fireLayout(q(container, 'toolbar-row'), 400)

        expect(q(container, 'toolbar-more')).not.toBeNull()
        fireEvent.click(q(container, 'toolbar-more-button') as Element)
        expect(rows(container)).toEqual(['Archive'])

        // Narrowing the row while the menu is open folds B in above the permanent row.
        fireLayout(q(container, 'toolbar-row'), 100)
        expect(rows(container)).toEqual(['B', 'Archive'])
    })

    it('re-measures when an item with a new key appears, and refits on resize', () => {
        const three = [button('a'), button('b'), button('c')]
        const { container, rerender, queryByRole } = renderBare(
            <OverlayProvider>
                <ResponsiveToolbar items={three} />
            </OverlayProvider>
        )
        for (const key of ['a', 'b', 'c']) fireLayout(q(container, `toolbar-item-${key}`), 40)
        fireLayout(q(container, 'toolbar-more'), 34)
        fireLayout(q(container, 'toolbar-row'), 162)
        expect((q(container, 'toolbar-row') as HTMLElement).style.opacity).toBe('1')

        rerender(
            <OverlayProvider>
                <ResponsiveToolbar items={[...three, button('d')]} />
            </OverlayProvider>
        )
        // Back to measuring: the unmeasured item is in the row, hidden.
        expect((q(container, 'toolbar-row') as HTMLElement).style.opacity).toBe('0')
        fireLayout(q(container, 'toolbar-item-d'), 40)
        expect((q(container, 'toolbar-row') as HTMLElement).style.opacity).toBe('1')
        expect(queryByRole('button', { name: 'D' })).toBeNull()

        fireLayout(q(container, 'toolbar-row'), 400)
        expect(queryByRole('button', { name: 'D' })).not.toBeNull()
        expect(q(container, 'toolbar-more')).toBeNull()
    })

    it('renders separators inline and folds them with their group', () => {
        const items: ToolbarItem[] = [
            button('a'),
            { type: 'separator' },
            { type: 'button', key: 'b', icon: Italic, label: 'B', onPress: () => {} },
            { type: 'button', key: 'c', icon: Underline, label: 'C', onPress: () => {} },
        ]
        const { container } = render(<ResponsiveToolbar items={items} />)
        for (const key of ['a', 'b', 'c']) fireLayout(q(container, `toolbar-item-${key}`), 40)
        fireLayout(q(container, 'toolbar-item-sep-1'), 9)
        fireLayout(q(container, 'toolbar-more'), 34)
        fireLayout(q(container, 'toolbar-row'), 80)

        expect(q(container, 'toolbar-item-sep-1')).toBeNull()
        fireEvent.click(q(container, 'toolbar-more-button') as Element)
        expect(rows(container)).toEqual(['B', 'C'])
    })
})
