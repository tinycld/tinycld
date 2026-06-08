import { describe, expect, it, vi } from 'vitest'
import { checkForUpdate } from '../client'
import type { UpdateManifest } from '../types'

const MANIFEST: UpdateManifest = {
    id: 'build-200-ios',
    runtimeVersion: '1.13.7',
    bundleUrl: '/api/app/bundle/build-200/ios/i.hbc',
    bundleHash: 'HASH',
    assets: [],
}

function deps(overrides = {}) {
    return {
        serverUrl: 'https://srv.test',
        platform: 'ios' as const,
        runtimeVersion: '1.13.7',
        currentId: 'build-100-ios',
        fetchFn: vi.fn(),
        ...overrides,
    }
}

describe('checkForUpdate', () => {
    it('returns null on 204 (up to date)', async () => {
        const fetchFn = vi.fn().mockResolvedValue({ status: 204, ok: true })
        const result = await checkForUpdate(deps({ fetchFn }))
        expect(result).toBeNull()
        expect(fetchFn).toHaveBeenCalledWith(
            'https://srv.test/api/app/update?platform=ios&runtimeVersion=1.13.7&currentId=build-100-ios'
        )
    })

    it('returns the manifest on 200', async () => {
        const fetchFn = vi.fn().mockResolvedValue({
            status: 200,
            ok: true,
            json: async () => MANIFEST,
        })
        const result = await checkForUpdate(deps({ fetchFn }))
        expect(result).toEqual(MANIFEST)
    })

    it('throws on a non-204 error status', async () => {
        const fetchFn = vi.fn().mockResolvedValue({ status: 500, ok: false })
        await expect(checkForUpdate(deps({ fetchFn }))).rejects.toThrow(/500/)
    })
})
