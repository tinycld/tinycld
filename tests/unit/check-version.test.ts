import { useVersionStore } from '@tinycld/core/lib/stores/version-store'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

async function importCheckVersion() {
    return await import('@tinycld/core/lib/check-version')
}

function mockFetchOk(releaseId: string) {
    return vi.fn(async () => ({
        ok: true,
        json: async () => ({ releaseId }),
    })) as unknown as typeof fetch
}

describe('checkVersion', () => {
    beforeEach(() => {
        useVersionStore.setState({ newVersionAvailable: false })
    })

    afterEach(() => {
        vi.restoreAllMocks()
    })

    it('does not flip the flag when server release matches boot release', async () => {
        const { checkVersion } = await importCheckVersion()
        await checkVersion({
            bootReleaseId: 'A',
            setNewVersionAvailable: useVersionStore.getState().setNewVersionAvailable,
            fetch: mockFetchOk('A'),
        })
        expect(useVersionStore.getState().newVersionAvailable).toBe(false)
    })

    it('flips the flag when server release differs from boot release', async () => {
        const { checkVersion } = await importCheckVersion()
        await checkVersion({
            bootReleaseId: 'A',
            setNewVersionAvailable: useVersionStore.getState().setNewVersionAvailable,
            fetch: mockFetchOk('B'),
        })
        expect(useVersionStore.getState().newVersionAvailable).toBe(true)
    })

    it('does not flip the flag on network error', async () => {
        const { checkVersion } = await importCheckVersion()
        const failingFetch = vi.fn(async () => {
            throw new Error('network fail')
        }) as unknown as typeof fetch
        await checkVersion({
            bootReleaseId: 'A',
            setNewVersionAvailable: useVersionStore.getState().setNewVersionAvailable,
            fetch: failingFetch,
        })
        expect(useVersionStore.getState().newVersionAvailable).toBe(false)
    })

    it('does not flip the flag when fetch returns non-ok response', async () => {
        const { checkVersion } = await importCheckVersion()
        const notOkFetch = vi.fn(async () => ({
            ok: false,
            json: async () => ({ releaseId: 'B' }),
        })) as unknown as typeof fetch
        await checkVersion({
            bootReleaseId: 'A',
            setNewVersionAvailable: useVersionStore.getState().setNewVersionAvailable,
            fetch: notOkFetch,
        })
        expect(useVersionStore.getState().newVersionAvailable).toBe(false)
    })

    it('does not flip the flag when response has no releaseId', async () => {
        const { checkVersion } = await importCheckVersion()
        const emptyFetch = vi.fn(async () => ({
            ok: true,
            json: async () => ({}),
        })) as unknown as typeof fetch
        await checkVersion({
            bootReleaseId: 'A',
            setNewVersionAvailable: useVersionStore.getState().setNewVersionAvailable,
            fetch: emptyFetch,
        })
        expect(useVersionStore.getState().newVersionAvailable).toBe(false)
    })
})
