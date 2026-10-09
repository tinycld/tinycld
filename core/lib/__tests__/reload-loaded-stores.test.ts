import { describe, expect, it, vi } from 'vitest'
import { reloadLoadedStores } from '../reload-loaded-stores'

function store(status: string) {
    return { status, reload: vi.fn(() => Promise.resolve()) }
}

describe('reloadLoadedStores', () => {
    it('reloads syncing stores and leaves idle and torn-down ones alone', async () => {
        const stores = {
            ready: store('ready'),
            loading: store('loading'),
            idle: store('idle'),
            cleanedUp: store('cleaned-up'),
        }

        await reloadLoadedStores(Object.values(stores))

        expect(stores.ready.reload).toHaveBeenCalledOnce()
        expect(stores.loading.reload).toHaveBeenCalledOnce()
        expect(stores.idle.reload).not.toHaveBeenCalled()
        expect(stores.cleanedUp.reload).not.toHaveBeenCalled()
    })

    it('resolves once every reload has settled', async () => {
        let settled = false
        const slow = {
            status: 'ready',
            reload: () =>
                new Promise<void>(resolve =>
                    setTimeout(() => {
                        settled = true
                        resolve()
                    }, 5)
                ),
        }

        await reloadLoadedStores([slow])

        expect(settled).toBe(true)
    })
})
