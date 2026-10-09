// @vitest-environment happy-dom
import { act, renderHook } from '@testing-library/react'
import { useConnectivityStore } from '@tinycld/core/lib/stores/connectivity-store'
import { useConnectivityDetector } from '@tinycld/core/lib/use-connectivity-detector.web'
import { afterEach, describe, expect, it } from 'vitest'

// The connection notice applies its own anti-flicker delay before it shows a
// problem. A second delay here would make the browser's offline state take
// both delays to reach the notice, so the store must take the browser's word
// at once.
describe('useConnectivityDetector (web)', () => {
    afterEach(() => useConnectivityStore.setState({ isOnline: true }))

    it('goes offline as soon as the browser says so, and back online the same way', () => {
        const { unmount } = renderHook(() => useConnectivityDetector())
        expect(useConnectivityStore.getState().isOnline).toBe(true)

        act(() => {
            window.dispatchEvent(new Event('offline'))
        })
        expect(useConnectivityStore.getState().isOnline).toBe(false)

        act(() => {
            window.dispatchEvent(new Event('online'))
        })
        expect(useConnectivityStore.getState().isOnline).toBe(true)
        unmount()
    })
})
