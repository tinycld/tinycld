import { type RetryConnectionDeps, retryConnection } from '@tinycld/core/lib/retry-connection'
import { describe, expect, it, vi } from 'vitest'

function deps(overrides: Partial<RetryConnectionDeps> = {}) {
    return {
        address: 'https://cloud.example.org',
        probe: vi.fn(async (_address: string) => {}),
        reload: vi.fn(async () => {}),
        setServerReachable: vi.fn(),
        onReloadError: vi.fn(),
        ...overrides,
    }
}

describe('retryConnection', () => {
    it('checks the server, reloads the live collections and marks the server reachable', async () => {
        const d = deps()
        await retryConnection(d)
        expect(d.probe).toHaveBeenCalledWith('https://cloud.example.org')
        expect(d.reload).toHaveBeenCalledTimes(1)
        expect(d.setServerReachable).toHaveBeenCalledWith(true)
    })

    it('does not wait for the reload, which waits out retries while the server is down', async () => {
        const d = deps({ reload: vi.fn(() => new Promise<void>(() => {})) })
        await expect(retryConnection(d)).resolves.toBeUndefined()
        expect(d.setServerReachable).toHaveBeenCalledWith(true)
    })

    it('rejects and leaves the server unreachable when the health check fails', async () => {
        const d = deps({ probe: vi.fn(async () => Promise.reject(new Error('down'))) })
        await expect(retryConnection(d)).rejects.toThrow('down')
        expect(d.reload).toHaveBeenCalledTimes(1)
        expect(d.setServerReachable).not.toHaveBeenCalled()
    })

    it('rejects when no server is chosen', async () => {
        const d = deps({ address: null })
        await expect(retryConnection(d)).rejects.toThrow('No server is chosen.')
        expect(d.probe).not.toHaveBeenCalled()
    })

    it('reports a failed reload instead of dropping it', async () => {
        const error = new Error('reload failed')
        const d = deps({ reload: vi.fn(async () => Promise.reject(error)) })
        await retryConnection(d)
        await vi.waitFor(() => expect(d.onReloadError).toHaveBeenCalledWith(error))
    })
})
