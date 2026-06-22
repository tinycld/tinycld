import { beforeEach, describe, expect, it, vi } from 'vitest'

// Mock the module graph use-app-updates pulls in. We only care about the
// reportRevertedBundle gate ordering, so the rest are inert stubs.
const takeRevertedBundle = vi.fn()
vi.mock('app-updater', () => ({
    default: {
        takeRevertedBundle: () => takeRevertedBundle(),
        getCurrentBundleId: () => 'build-1-ios',
        getRuntimeVersion: () => '2.0.0',
    },
}))

const getResolvedAddress = vi.fn<() => string | null>()
vi.mock('@tinycld/core/lib/server-address', () => ({
    getResolvedAddress: () => getResolvedAddress(),
}))

// PB_SERVER_ADDR must NOT be touched by reportRevertedBundle (that was the bug).
// Make the proxy throw on ANY access so the test fails loudly if it's read.
vi.mock('@tinycld/core/lib/config', () => ({
    get PB_SERVER_ADDR(): string {
        throw new Error('PB_SERVER_ADDR accessed before server address was resolved')
    },
}))

const reportBadBundle = vi.fn().mockResolvedValue(undefined)
vi.mock('@tinycld/core/lib/app-updater/client', () => ({
    reportBadBundle: (...args: unknown[]) => reportBadBundle(...args),
    isUpdateTransportAllowed: (url: string) => url.startsWith('https://'),
    checkForUpdate: vi.fn(),
    downloadAndStage: vi.fn(),
}))

const captureException = vi.fn()
vi.mock('@tinycld/core/lib/errors', () => ({ captureException: (...a: unknown[]) => captureException(...a) }))
vi.mock('@tinycld/core/lib/app-updater/hash', () => ({ sha256HexOfFile: vi.fn() }))
vi.mock('@tinycld/core/lib/stores/toast-store', () => ({ useToastStore: { getState: () => ({ addToast: vi.fn() }) } }))
vi.mock('expo-file-system/legacy', () => ({ documentDirectory: 'file:///', makeDirectoryAsync: vi.fn(), downloadAsync: vi.fn() }))
vi.mock('react-native', () => ({
    Platform: { OS: 'ios' },
    AppState: { addEventListener: () => ({ remove: () => {} }) },
}))

// __DEV__ must be false for the native path to run.
;(globalThis as { __DEV__?: boolean }).__DEV__ = false

import { reportRevertedBundle } from '../../use-app-updates'

describe('reportRevertedBundle', () => {
    beforeEach(() => {
        vi.clearAllMocks()
    })

    // The regression guard: before the server-address gate resolves,
    // getResolvedAddress() is null. The function must bail WITHOUT consuming the
    // reverted marker (so a later foreground retries) and WITHOUT throwing or
    // reporting an error. The original bug consumed the marker then threw on
    // PB_SERVER_ADDR, losing the report and spamming Sentry.
    it('does not consume the marker or report when the address is unresolved', async () => {
        getResolvedAddress.mockReturnValue(null)

        await expect(reportRevertedBundle()).resolves.toBeUndefined()

        expect(takeRevertedBundle).not.toHaveBeenCalled() // marker preserved for retry
        expect(reportBadBundle).not.toHaveBeenCalled()
        expect(captureException).not.toHaveBeenCalled() // no Sentry spam
    })

    it('reports the reverted bundle once the address resolves', async () => {
        getResolvedAddress.mockReturnValue('https://srv.test')
        takeRevertedBundle.mockReturnValue({ id: 'build-1-ios', hash: 'HASH' })

        await reportRevertedBundle()

        expect(takeRevertedBundle).toHaveBeenCalledTimes(1)
        expect(reportBadBundle).toHaveBeenCalledWith(
            expect.objectContaining({ id: 'build-1-ios', hash: 'HASH', platform: 'ios' })
        )
    })

    it('no-ops (no report) when there is no reverted bundle', async () => {
        getResolvedAddress.mockReturnValue('https://srv.test')
        takeRevertedBundle.mockReturnValue(null)

        await reportRevertedBundle()

        expect(reportBadBundle).not.toHaveBeenCalled()
    })

    // Insecure transport (plain http://) must also bail before consuming the marker.
    it('does not consume the marker over insecure transport', async () => {
        getResolvedAddress.mockReturnValue('http://srv.test')

        await reportRevertedBundle()

        expect(takeRevertedBundle).not.toHaveBeenCalled()
    })
})
