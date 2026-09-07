// @vitest-environment happy-dom

import { cleanup, render as renderBare } from '@testing-library/react'
import { OverlayProvider } from '@tinycld/core/ui/overlay'
import { Sheet } from '@tinycld/core/ui/sheet'
import type { ReactElement } from 'react'
import { Text } from 'react-native'
import { afterEach, describe, expect, it } from 'vitest'

const render = (ui: ReactElement) => renderBare(<OverlayProvider>{ui}</OverlayProvider>)

const surfaceOf = (root: ParentNode) => {
    const surface = root.querySelector<HTMLElement>('[testid="sheet"]')
    if (!surface) throw new Error('sheet surface not rendered')
    return surface
}

// The drag pill sits at the edge the user pulls away from: last child of a
// top sheet, first of a bottom one.
const handleIsAfterContent = (surface: HTMLElement) => {
    const handle = surface.querySelector('[testid="sheet-handle"]')
    const content = surface.querySelector('[testid="content"]')
    if (!handle || !content) throw new Error('handle or content missing')
    return Boolean(content.compareDocumentPosition(handle) & Node.DOCUMENT_POSITION_FOLLOWING)
}

describe('Sheet side', () => {
    afterEach(cleanup)

    it('rests on the bottom edge by default, pill first', () => {
        const { container } = render(
            <Sheet isOpen onClose={() => {}} testID="sheet">
                <Text testID="content">Body</Text>
            </Sheet>
        )
        const surface = surfaceOf(container)
        expect(surface.className).toContain('bottom-0')
        expect(surface.className).toContain('rounded-t-2xl')
        expect(surface.className).not.toContain('top-0')
        expect(handleIsAfterContent(surface)).toBe(false)
    })

    it('hangs from the top edge with side="top", pill last', () => {
        const { container } = render(
            <Sheet isOpen onClose={() => {}} side="top" testID="sheet">
                <Text testID="content">Body</Text>
            </Sheet>
        )
        const surface = surfaceOf(container)
        expect(surface.className).toContain('top-0')
        expect(surface.className).toContain('rounded-b-2xl')
        expect(surface.className).not.toContain('bottom-0')
        expect(handleIsAfterContent(surface)).toBe(true)
    })
})
