// @vitest-environment happy-dom
import { beforeEach, describe, expect, it, vi } from 'vitest'

const postBootBeacon = vi.hoisted(() => vi.fn())
const getResolvedAddress = vi.hoisted(() => vi.fn<() => string | null>())
vi.mock('@tinycld/core/lib/app-updater/client', () => ({
    postBootBeacon,
    isUpdateTransportAllowed: (url: string) =>
        url.startsWith('https://') || url.startsWith('http://localhost'),
}))
vi.mock('@tinycld/core/lib/server-address', () => ({
    getResolvedAddress: () => getResolvedAddress(),
}))
vi.mock('@tinycld/core/lib/errors', () => ({ captureException: vi.fn() }))
vi.mock('app-updater', () => ({
    default: {
        getCurrentBundleId: () => 'build-9-ios',
        getCurrentBundleHash: () => 'deadbeefcafe',
    },
}))
// The react-native unit stub reports Platform.OS === 'web', which would make the
// hook early-return and never post. Override Platform to 'ios' so these tests
// genuinely exercise the beacon path.
vi.mock('react-native', () => ({ Platform: { OS: 'ios' } }))

import { renderHook } from '@testing-library/react'
import { useBundleSentinel } from '@tinycld/core/lib/bundle-sentinel'

describe('useBundleSentinel', () => {
    beforeEach(() => {
        postBootBeacon.mockReset().mockResolvedValue(undefined)
        getResolvedAddress.mockReset()
    })

    it('posts a boot beacon with the running bundle id once the server address is resolved', () => {
        getResolvedAddress.mockReturnValue('http://localhost:7090')
        renderHook(() => useBundleSentinel())
        expect(postBootBeacon).toHaveBeenCalledOnce()
        expect(postBootBeacon).toHaveBeenCalledWith(
            expect.objectContaining({
                serverUrl: 'http://localhost:7090',
                platform: 'ios',
                id: 'build-9-ios',
                hash: 'deadbeefcafe',
            })
        )
    })

    it('does not post when no server address is resolved', () => {
        getResolvedAddress.mockReturnValue(null)
        renderHook(() => useBundleSentinel())
        expect(postBootBeacon).not.toHaveBeenCalled()
    })

    it('does not post when the transport is not allowed', () => {
        // A non-loopback http:// address fails the transport gate (the mocked
        // isUpdateTransportAllowed only permits https:// or http://localhost).
        getResolvedAddress.mockReturnValue('http://192.168.1.10:7090')
        renderHook(() => useBundleSentinel())
        expect(postBootBeacon).not.toHaveBeenCalled()
    })
})
