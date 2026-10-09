// @vitest-environment happy-dom

import { act, renderHook } from '@testing-library/react'
import { useConnectivityStore } from '@tinycld/core/lib/stores/connectivity-store'
import { useServerActionState } from '@tinycld/core/lib/use-writes-available'
import { beforeEach, describe, expect, it } from 'vitest'

const OFFLINE = "You're offline — changes can't be saved right now"

describe('useServerActionState', () => {
    beforeEach(() => useConnectivityStore.setState({ isOnline: true, isServerReachable: true }))

    it('passes through the caller’s own state while writes are available', () => {
        const { result } = renderHook(() =>
            useServerActionState({ isDisabled: false, accessibilityHint: 'Removes the member' })
        )
        expect(result.current).toEqual({
            isDisabled: false,
            accessibilityHint: 'Removes the member',
        })
    })

    it('keeps the caller’s own disabled state while writes are available', () => {
        const { result } = renderHook(() => useServerActionState({ isDisabled: true }))
        expect(result.current.isDisabled).toBe(true)
    })

    it('disables and overrides the hint during an outage', () => {
        act(() => useConnectivityStore.setState({ isOnline: false }))
        const { result } = renderHook(() =>
            useServerActionState({ isDisabled: false, accessibilityHint: 'Removes the member' })
        )
        expect(result.current).toEqual({ isDisabled: true, accessibilityHint: OFFLINE })
    })

    it('defaults to enabled with no hint when called with no args', () => {
        const { result } = renderHook(() => useServerActionState())
        expect(result.current).toEqual({ isDisabled: false, accessibilityHint: undefined })
    })
})
