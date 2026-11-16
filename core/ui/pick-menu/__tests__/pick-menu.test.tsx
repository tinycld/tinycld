// @vitest-environment happy-dom
import { cleanup, fireEvent, render } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { OverlayProvider } from '../../overlay'
import { PickMenu } from '../index'

afterEach(cleanup)

// RN stub hosts carry `testID`; Menu rows are DOM elements with `data-testid`
// under the overlay host, outside the render container.
const q = (id: string) =>
    document.body.querySelector(`[testID="${id}"], [data-testid="${id}"]`) as HTMLElement | null

describe('PickMenu', () => {
    it('opens on the trigger and selects an item', () => {
        const onSelect = vi.fn()
        render(
            <OverlayProvider>
                <PickMenu
                    label="Bug"
                    purpose="Type"
                    testID="t"
                    items={[
                        { id: 'a', label: 'Bug', isSelected: true, onSelect: vi.fn() },
                        { id: 'b', label: 'Task', onSelect },
                    ]}
                />
            </OverlayProvider>
        )
        const trigger = q('t') as HTMLElement
        expect(
            trigger.getAttribute('accessibilityLabel') ?? trigger.getAttribute('aria-label')
        ).toBe('Type: Bug')
        fireEvent.click(trigger)
        fireEvent.click(q('t-b') as HTMLElement)
        expect(onSelect).toHaveBeenCalledTimes(1)
    })

    it('reports a disabled chip as disabled and opens no menu on a press', () => {
        const onSelect = vi.fn()
        render(
            <OverlayProvider>
                <PickMenu
                    label="Bug"
                    purpose="Type"
                    testID="t"
                    isDisabled
                    items={[
                        { id: 'a', label: 'Bug', isSelected: true, onSelect: vi.fn() },
                        { id: 'b', label: 'Task', onSelect },
                    ]}
                />
            </OverlayProvider>
        )
        const trigger = q('t') as HTMLElement
        expect(trigger.getAttribute('aria-disabled')).toBe('true')
        fireEvent.click(trigger)
        expect(q('t-a')).toBeNull()
        expect(q('t-b')).toBeNull()
        expect(onSelect).not.toHaveBeenCalled()
    })

    it('keeps a chip with isDisabled false enabled and opening its menu', () => {
        const onSelect = vi.fn()
        render(
            <OverlayProvider>
                <PickMenu
                    label="Bug"
                    purpose="Type"
                    testID="t"
                    isDisabled={false}
                    items={[
                        { id: 'a', label: 'Bug', isSelected: true, onSelect: vi.fn() },
                        { id: 'b', label: 'Task', onSelect },
                    ]}
                />
            </OverlayProvider>
        )
        const trigger = q('t') as HTMLElement
        expect(trigger.getAttribute('aria-disabled')).not.toBe('true')
        fireEvent.click(trigger)
        fireEvent.click(q('t-b') as HTMLElement)
        expect(onSelect).toHaveBeenCalledTimes(1)
    })

    it('offers no live choice from a menu that was open when the chip became disabled', () => {
        const onSelectA = vi.fn()
        const onSelectB = vi.fn()
        const chip = (isDisabled: boolean) => (
            <OverlayProvider>
                <PickMenu
                    label="Bug"
                    purpose="Type"
                    testID="t"
                    isDisabled={isDisabled}
                    items={[
                        { id: 'a', label: 'Bug', isSelected: true, onSelect: onSelectA },
                        { id: 'b', label: 'Task', onSelect: onSelectB },
                    ]}
                />
            </OverlayProvider>
        )
        const { rerender } = render(chip(false))
        fireEvent.click(q('t') as HTMLElement)
        expect(q('t-b')).not.toBeNull()
        rerender(chip(true))
        // A row may stay drawn or the menu may close; either way no press may pick.
        for (const id of ['t-b', 't-a']) {
            const row = q(id)
            if (row) fireEvent.click(row)
        }
        expect(onSelectB).not.toHaveBeenCalled()
        expect(onSelectA).not.toHaveBeenCalled()
    })

    it('gives the touch size a 44pt minimum height', () => {
        render(
            <OverlayProvider>
                <PickMenu
                    label="Bug"
                    purpose="Type"
                    testID="t"
                    size="touch"
                    items={[{ id: 'a', label: 'Bug', onSelect: vi.fn() }]}
                />
            </OverlayProvider>
        )
        expect((q('t') as HTMLElement).style.minHeight).toBe('44px')
    })
})
