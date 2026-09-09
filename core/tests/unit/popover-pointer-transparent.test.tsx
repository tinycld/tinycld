// @vitest-environment happy-dom

import { cleanup, render as renderBare } from '@testing-library/react'
import { OverlayProvider } from '@tinycld/core/ui/overlay'
import { Popover } from '@tinycld/core/ui/popover'
import type { ReactElement } from 'react'
import { Text } from 'react-native'
import { afterEach, describe, expect, it } from 'vitest'

const render = (ui: ReactElement) => renderBare(<OverlayProvider>{ui}</OverlayProvider>)

// The stub renders testID and pointerEvents as plain attributes.
function surface(): Element {
    const node = document.querySelector('[testid="surf"]')
    if (!node) throw new Error('no popover surface rendered')
    return node
}

/**
 * A hover tooltip is placed against the very element the pointer is on, so an
 * interactive surface lands between a press and its release and swallows the
 * mouseup — the control under it silently never fires. Only such a surface
 * opts out of pointer events; everything with its own controls needs them.
 */
describe('Popover pointerTransparent', () => {
    afterEach(cleanup)

    it('lets presses through when set', () => {
        render(
            <Popover anchor={{ x: 10, y: 10 }} isOpen testID="surf" pointerTransparent>
                <Text>who reacted</Text>
            </Popover>
        )
        expect(surface().getAttribute('pointerevents')).toBe('none')
    })

    it('captures presses by default, so a menu keeps its own', () => {
        render(
            <Popover anchor={{ x: 10, y: 10 }} isOpen testID="surf">
                <Text>a menu item</Text>
            </Popover>
        )
        expect(surface().getAttribute('pointerevents')).toBe('auto')
    })
})
