import { describe, expect, it, vi } from 'vitest'
import { refetchLoadedStores } from '../refetch-loaded-stores'

function store(status: string) {
    return { status, utils: { refetch: vi.fn(() => Promise.resolve()) } }
}

describe('refetchLoadedStores', () => {
    it('fetches syncing stores again and leaves idle and torn-down ones alone', async () => {
        const stores = {
            ready: store('ready'),
            loading: store('loading'),
            idle: store('idle'),
            cleanedUp: store('cleaned-up'),
        }

        await refetchLoadedStores(Object.values(stores))

        expect(stores.ready.utils.refetch).toHaveBeenCalledOnce()
        expect(stores.loading.utils.refetch).toHaveBeenCalledOnce()
        expect(stores.idle.utils.refetch).not.toHaveBeenCalled()
        expect(stores.cleanedUp.utils.refetch).not.toHaveBeenCalled()
    })

    it('resolves once every refetch has settled', async () => {
        let settled = false
        const slow = {
            status: 'ready',
            utils: {
                refetch: () =>
                    new Promise<void>(resolve =>
                        setTimeout(() => {
                            settled = true
                            resolve()
                        }, 5)
                    ),
            },
        }

        await refetchLoadedStores([slow])

        expect(settled).toBe(true)
    })
})
