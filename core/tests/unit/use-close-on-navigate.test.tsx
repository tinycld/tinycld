// @vitest-environment happy-dom
import { renderHook } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

// Drive usePathname from a mutable module-level value so each test can simulate
// navigation by changing it between renders. This local mock overrides the
// constant '/' usePathname from unit-setup.ts for this file only.
let pathname = '/a/acme/drive'
vi.mock('expo-router', () => ({ usePathname: () => pathname }))

import { useCloseOnNavigate } from '../../ui/drawer/use-close-on-navigate'

afterEach(() => {
    pathname = '/a/acme/drive'
})

test('does not close on the initial mount (opened on the current route)', () => {
    const onClose = vi.fn()
    renderHook(() => useCloseOnNavigate(true, onClose))
    expect(onClose).not.toHaveBeenCalled()
})

test('closes when the route changes while open', () => {
    const onClose = vi.fn()
    const { rerender } = renderHook(() => useCloseOnNavigate(true, onClose))
    expect(onClose).not.toHaveBeenCalled()

    pathname = '/a/acme/mail'
    rerender()
    expect(onClose).toHaveBeenCalledTimes(1)
})

test('does not close while the drawer is shut, even across navigation', () => {
    const onClose = vi.fn()
    const { rerender } = renderHook(({ isOpen }) => useCloseOnNavigate(isOpen, onClose), {
        initialProps: { isOpen: false },
    })

    pathname = '/a/acme/mail'
    rerender({ isOpen: false })
    expect(onClose).not.toHaveBeenCalled()
})

test('re-baselines on reopen: opening on a new route does not immediately close', () => {
    const onClose = vi.fn()
    const { rerender } = renderHook(({ isOpen }) => useCloseOnNavigate(isOpen, onClose), {
        initialProps: { isOpen: true },
    })

    // Close the drawer, then navigate elsewhere while closed.
    rerender({ isOpen: false })
    pathname = '/a/acme/calendar'
    rerender({ isOpen: false })
    expect(onClose).not.toHaveBeenCalled()

    // Reopening on the new route must treat it as the fresh baseline, not fire.
    rerender({ isOpen: true })
    expect(onClose).not.toHaveBeenCalled()

    // ...and a subsequent navigation away from that route still closes it.
    pathname = '/a/acme/contacts'
    rerender({ isOpen: true })
    expect(onClose).toHaveBeenCalledTimes(1)
})

test('tolerates a missing onClose', () => {
    const { rerender } = renderHook(() => useCloseOnNavigate(true, undefined))
    pathname = '/a/acme/mail'
    expect(() => rerender()).not.toThrow()
})

test('uses the latest onClose without re-arming on identity change', () => {
    const first = vi.fn()
    const second = vi.fn()
    const { rerender } = renderHook(({ cb }) => useCloseOnNavigate(true, cb), {
        initialProps: { cb: first },
    })

    // Swap the callback identity without navigating — must not fire either one.
    rerender({ cb: second })
    expect(first).not.toHaveBeenCalled()
    expect(second).not.toHaveBeenCalled()

    // Now navigate: only the latest callback runs.
    pathname = '/a/acme/mail'
    rerender({ cb: second })
    expect(first).not.toHaveBeenCalled()
    expect(second).toHaveBeenCalledTimes(1)
})
