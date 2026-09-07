// @vitest-environment happy-dom

import { cleanup, render } from '@testing-library/react'
import { ToastRenderer } from '@tinycld/core/components/Toast'
import { useToastStore } from '@tinycld/core/lib/stores/toast-store'
import { useWindowSizeStore } from '@tinycld/core/lib/stores/window-size-store'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('react-native-safe-area-context', () => ({
    useSafeAreaInsets: () => ({ top: 0, right: 0, bottom: 0, left: 0 }),
}))

/**
 * Each toast card is an opaque surface, so whatever is under it is unclickable
 * for the card's lifetime. The stack used to pin top-right on web — the one
 * region every package fills with header actions — and a boards spec caught a
 * 6s "Sprint completed" card sitting on the sprint-scope pill it was about to
 * click. Desktop web now pins bottom-left, past the package rail; mobile-width
 * web keeps the top banner so it stays clear of the mobile tab bar.
 */
describe('ToastRenderer placement (web)', () => {
    beforeEach(() => {
        useToastStore.setState({ toasts: [] })
        useToastStore
            .getState()
            .addToast({ title: 'Sprint 1 completed', variant: 'success', duration: 6000 })
    })

    afterEach(() => {
        cleanup()
        useToastStore.setState({ toasts: [] })
    })

    function stackStyle(width: number) {
        useWindowSizeStore.setState({ width, height: 800 })
        const { container } = render(<ToastRenderer />)
        const stack = container.querySelector('[testid="toast-stack"]') as HTMLElement | null
        if (!stack) throw new Error('toast stack did not render')
        return stack.style
    }

    it('pins bottom-left, clear of the 64px rail, at desktop width', () => {
        const style = stackStyle(1280)
        expect(style.bottom).toBe('16px')
        expect(style.left).toBe('80px')
        expect(style.top).toBe('')
        expect(style.right).toBe('')
    })

    it('pins bottom-left at tablet width too', () => {
        const style = stackStyle(900)
        expect(style.bottom).toBe('16px')
        expect(style.top).toBe('')
    })

    it('keeps the top banner at mobile width', () => {
        const style = stackStyle(375)
        expect(style.top).toBe('16px')
        expect(style.bottom).toBe('')
    })
})
