import { describe, expect, it, vi } from 'vitest'
import { checkForUpdate, downloadAndStage } from '../client'
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

function stageDeps(overrides = {}) {
    return {
        serverUrl: 'https://srv.test',
        downloadFn: vi.fn().mockResolvedValue({ uri: 'file:///tmp/i.hbc' }),
        hashFn: vi.fn().mockResolvedValue('HASH'), // matches MANIFEST.bundleHash
        stageBundleFn: vi.fn().mockResolvedValue(undefined),
        tmpDir: 'file:///tmp/upd/',
        ...overrides,
    }
}

describe('downloadAndStage', () => {
    it('stages when the bundle hash matches', async () => {
        const d = stageDeps()
        await downloadAndStage(MANIFEST, d)
        expect(d.stageBundleFn).toHaveBeenCalledWith('file:///tmp/upd/', 'build-200-ios')
    })

    it('throws and does not stage on bundle hash mismatch', async () => {
        const d = stageDeps({ hashFn: vi.fn().mockResolvedValue('WRONG') })
        await expect(downloadAndStage(MANIFEST, d)).rejects.toThrow(/hash mismatch/)
        expect(d.stageBundleFn).not.toHaveBeenCalled()
    })

    it('downloads and verifies each asset, then stages', async () => {
        const manifestWithAsset = {
            ...MANIFEST,
            assets: [
                {
                    key: 'assets/a',
                    hash: 'AH',
                    contentType: 'image/png',
                    url: '/api/app/asset/build-200/ios/assets/a',
                },
            ],
        }
        // bundle hash 'HASH', asset hash 'AH' — hashFn returns per-call values
        const hashFn = vi
            .fn()
            .mockResolvedValueOnce('HASH') // bundle
            .mockResolvedValueOnce('AH') // asset
        const d = stageDeps({ hashFn })
        await downloadAndStage(manifestWithAsset, d)
        expect(d.downloadFn).toHaveBeenCalledTimes(2) // bundle + 1 asset
        expect(d.stageBundleFn).toHaveBeenCalledWith('file:///tmp/upd/', 'build-200-ios')
    })

    it('throws on asset hash mismatch and does not stage', async () => {
        const manifestWithAsset = {
            ...MANIFEST,
            assets: [
                {
                    key: 'assets/a',
                    hash: 'AH',
                    contentType: 'image/png',
                    url: '/api/app/asset/build-200/ios/assets/a',
                },
            ],
        }
        const hashFn = vi
            .fn()
            .mockResolvedValueOnce('HASH') // bundle ok
            .mockResolvedValueOnce('WRONG') // asset bad
        const d = stageDeps({ hashFn })
        await expect(downloadAndStage(manifestWithAsset, d)).rejects.toThrow(/hash mismatch/)
        expect(d.stageBundleFn).not.toHaveBeenCalled()
    })
})
