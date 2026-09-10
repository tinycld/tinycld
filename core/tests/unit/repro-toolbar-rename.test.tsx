// @vitest-environment happy-dom

import { cleanup, fireEvent, render as renderBare } from '@testing-library/react'
import { ResponsiveToolbar, type ToolbarItem } from '@tinycld/core/components/ResponsiveToolbar'
import { Menu } from '@tinycld/core/ui/menu'
import { OverlayProvider } from '@tinycld/core/ui/overlay'
import type { ReactElement } from 'react'
import { useState } from 'react'
import { Text, TextInput } from 'react-native'
import { afterEach, describe, expect, it } from 'vitest'

const render = (ui: ReactElement) => renderBare(<OverlayProvider>{ui}</OverlayProvider>)
const fireLayout = (el: Element | null, width: number) => {
    if (!el) throw new Error('no element to lay out')
    fireEvent(el, new CustomEvent('rn-layout', { detail: { x: 0, y: 0, width, height: 40 } }))
}
const q = (root: ParentNode, testId: string) => root.querySelector(`[testid="${testId}"]`)

/** The BoardHeader shape: a permanent More menu whose row flips state that a
 *  PINNED custom item renders from. */
function Harness() {
    const [isRenaming, setIsRenaming] = useState(false)
    const items: ToolbarItem[] = [
        {
            type: 'custom',
            key: 'title',
            minWidth: 160,
            element: isRenaming ? (
                <TextInput testID="name-input" value="Board" />
            ) : (
                <Text>Board</Text>
            ),
        },
    ]
    return (
        <ResponsiveToolbar
            items={items}
            moreMenu={
                <Menu.Item
                    label="Rename board"
                    testID="rename"
                    onSelect={() => setIsRenaming(true)}
                />
            }
            height={40}
            gap={12}
        />
    )
}

describe('toolbar menu row flipping a pinned item', () => {
    afterEach(cleanup)

    it('swaps the pinned title for the input when the menu row is chosen', () => {
        const { container, getByRole, baseElement } = render(<Harness />)
        fireLayout(q(container, 'toolbar-item-title'), 200)
        fireLayout(q(container, 'toolbar-more'), 34)
        fireLayout(q(container, 'toolbar-row'), 600)

        fireEvent.click(q(container, 'toolbar-more-button') as Element)
        fireEvent.click(getByRole('menuitem', { name: 'Rename board' }))

        expect(q(baseElement, 'name-input')).not.toBeNull()
    })
})
