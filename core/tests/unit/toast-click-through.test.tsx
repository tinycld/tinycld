// @vitest-environment happy-dom

import { cleanup, render } from '@testing-library/react'
import { ToastRenderer } from '@tinycld/core/components/Toast'
import { useToastStore } from '@tinycld/core/lib/stores/toast-store'
import { afterEach, describe, expect, it } from 'vitest'

/**
 * A toast sits over the top-right of every screen for its whole life. It must
 * not swallow the clicks meant for what it covers — a board header's controls
 * — so the card lets pointers through everywhere except its own targets: the
 * dismiss button and the optional action.
 */
describe('ToastRenderer', () => {
    afterEach(() => {
        cleanup()
        useToastStore.setState({ toasts: [] })
    })

    it('lets pointers fall through the card, except on its dismiss and action', () => {
        useToastStore.getState().addToast({
            title: 'Sprint 1 completed',
            body: '3 completed · 1 moved to Sprint 2',
            variant: 'success',
            duration: 4000,
            action: { label: 'Undo', onPress: () => {} },
        })
        const { container, getByText } = render(<ToastRenderer />)

        // The stub renders testID and accessibilityLabel as plain attributes.
        const card = container.querySelector('[testid="toast-card"]')
        if (!card) throw new Error('no toast card rendered')
        expect(card.getAttribute('pointerevents')).toBe('box-none')
        expect(getByText('Sprint 1 completed').getAttribute('pointerevents')).toBe('none')
        expect(getByText('3 completed · 1 moved to Sprint 2').getAttribute('pointerevents')).toBe(
            'none'
        )

        const dismiss = container.querySelector('[accessibilitylabel="Dismiss notification"]')
        expect(dismiss).not.toBeNull()
        expect(dismiss?.getAttribute('pointerevents')).toBeNull()
        expect(getByText('Undo').closest('rn-pressable')?.getAttribute('pointerevents')).toBeNull()
    })
})
