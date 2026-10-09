// @vitest-environment happy-dom
import { act, renderHook } from '@testing-library/react'
import { useConnectivityStore } from '@tinycld/core/lib/stores/connectivity-store'
import { useWritesAvailable } from '@tinycld/core/lib/use-writes-available'
import { writesAvailability } from '@tinycld/core/lib/writes-available'
import { beforeEach, describe, expect, it } from 'vitest'

describe('writesAvailability', () => {
    it('allows writes while online and the server answers', () => {
        expect(writesAvailability({ isOnline: true, isServerReachable: true })).toEqual({
            available: true,
            reason: '',
        })
    })

    it('blocks writes offline, and says so first', () => {
        expect(writesAvailability({ isOnline: false, isServerReachable: false })).toEqual({
            available: false,
            reason: "You're offline — changes can't be saved right now",
        })
    })

    it('blocks writes while the server is unreachable', () => {
        expect(writesAvailability({ isOnline: true, isServerReachable: false })).toEqual({
            available: false,
            reason: "Can't reach the server — changes can't be saved right now",
        })
    })
})

describe('useWritesAvailable', () => {
    beforeEach(() => useConnectivityStore.setState({ isOnline: true, isServerReachable: true }))

    it('follows the connectivity store', () => {
        const { result } = renderHook(() => useWritesAvailable())
        expect(result.current.available).toBe(true)

        act(() => useConnectivityStore.setState({ isOnline: false }))
        expect(result.current.available).toBe(false)

        act(() => useConnectivityStore.setState({ isOnline: true, isServerReachable: false }))
        expect(result.current.reason).toBe(
            "Can't reach the server — changes can't be saved right now"
        )

        act(() => useConnectivityStore.setState({ isServerReachable: true }))
        expect(result.current).toEqual({ available: true, reason: '' })
    })
})
