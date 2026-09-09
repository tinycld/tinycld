// @vitest-environment happy-dom
import { beforeEach, describe, expect, it, vi } from 'vitest'

// Web push was dead in a way no test could catch: web-push.ts read the VAPID
// public key from `process.env.VITE_VAPID_PUBLIC_KEY`, a name nothing in the
// repo ever defined (VITE_* is Vite's prefix; this app bundles with Metro,
// which only inlines EXPO_PUBLIC_*). subscribeToPush therefore returned false
// before ever calling pushManager.subscribe, so the Settings toggle silently
// did nothing and no push_subscriptions row was written.
//
// The key is runtime, not build-time, config: each deployment generates its own
// keypair in Setup → Settings, and the server injects the public half into the
// page (coreserver/static.go::publicConfigScript). These tests pin that
// contract — the key comes from core config, and a missing one is LOUD.

const getCoreConfigOptional = vi.hoisted(() => vi.fn())
vi.mock('@tinycld/core/lib/core-config', () => ({ getCoreConfigOptional }))

import { isPushConfigured, subscribeToPush, vapidPublicKey } from '@tinycld/core/lib/web-push'

// isPushSupported() requires serviceWorker + PushManager; happy-dom has
// neither, so stub them to reach the key-resolution path under test.
function stubBrowserPushSupport() {
    Object.defineProperty(navigator, 'serviceWorker', {
        value: { register: vi.fn() },
        configurable: true,
    })
    ;(window as unknown as { PushManager: unknown }).PushManager = () => {}
}

describe('web push VAPID key resolution', () => {
    beforeEach(() => {
        vi.clearAllMocks()
        stubBrowserPushSupport()
    })

    it('reads the key from core config, not process.env', () => {
        getCoreConfigOptional.mockReturnValue({ vapidPublicKey: 'BKey123' })
        expect(vapidPublicKey()).toBe('BKey123')
    })

    it('reports an unconfigured server as not-configured even when supported', () => {
        getCoreConfigOptional.mockReturnValue({})
        expect(vapidPublicKey()).toBe('')
        expect(isPushConfigured()).toBe(false)
    })

    it('is configured once the operator has generated a keypair', () => {
        getCoreConfigOptional.mockReturnValue({ vapidPublicKey: 'BKey123' })
        expect(isPushConfigured()).toBe(true)
    })

    it('tolerates configureCore never having run', () => {
        getCoreConfigOptional.mockReturnValue(null)
        expect(vapidPublicKey()).toBe('')
    })

    // The regression that hid the original bug: a silent `return false` is
    // indistinguishable from "unsupported", so the UI showed a dead toggle and
    // usePushSubscription's onError never fired. It must throw instead.
    it('throws rather than silently failing when no key is configured', async () => {
        getCoreConfigOptional.mockReturnValue({})
        await expect(subscribeToPush('user-1')).rejects.toThrow(/not configured/i)
    })
})
