import { describe, expect, it, vi } from 'vitest'
import {
    checkForUpdate,
    downloadAndStage,
    isUpdateTransportAllowed,
    postBootBeacon,
    reportBadBundle,
} from '../client'
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
        currentHash: 'CURHASH',
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
            'https://srv.test/api/app/update?platform=ios&runtimeVersion=1.13.7&currentId=build-100-ios&currentHash=CURHASH'
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
        platform: 'ios' as const,
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
        expect(d.stageBundleFn).toHaveBeenCalledWith('file:///tmp/upd/', 'build-200-ios', 'HASH')
    })

    it('downloads into native/<platform>/ matching the native locateHbc layout', async () => {
        // The native module searches `<stagedDir>/native/<platform>/` for the
        // .hbc. If the download layout drifts from that, the staged bundle is
        // never found and the update silently reverts to embedded. This pins the
        // contract: bundleUrl .../ios/i.hbc → file:///tmp/upd/native/ios/i.hbc.
        const d = stageDeps()
        await downloadAndStage(MANIFEST, d)
        expect(d.downloadFn).toHaveBeenCalledWith(
            'https://srv.test/api/app/bundle/build-200/ios/i.hbc',
            'file:///tmp/upd/native/ios/i.hbc'
        )
    })

    it('lays assets out under native/<platform>/ at their server-relative path', async () => {
        const manifestWithAsset = {
            ...MANIFEST,
            assets: [
                {
                    key: 'assets/a',
                    hash: 'AH',
                    contentType: 'image/png',
                    url: '/api/app/asset/build-200/ios/assets/img/a.png',
                },
            ],
        }
        const hashFn = vi.fn().mockResolvedValueOnce('HASH').mockResolvedValueOnce('AH')
        const d = stageDeps({ hashFn })
        await downloadAndStage(manifestWithAsset, d)
        expect(d.downloadFn).toHaveBeenCalledWith(
            'https://srv.test/api/app/asset/build-200/ios/assets/img/a.png',
            'file:///tmp/upd/native/ios/assets/img/a.png'
        )
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
        expect(d.stageBundleFn).toHaveBeenCalledWith('file:///tmp/upd/', 'build-200-ios', 'HASH')
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

describe('reportBadBundle', () => {
    it('POSTs the bundle id/hash/platform/error to report-bad', async () => {
        const fetchFn = vi.fn().mockResolvedValue({ ok: true, status: 200 })
        await reportBadBundle({
            serverUrl: 'https://srv.test',
            platform: 'ios',
            id: 'build-200-ios',
            hash: 'HASH',
            error: 'rolled back',
            fetchFn,
        })
        expect(fetchFn).toHaveBeenCalledWith(
            'https://srv.test/api/app/update/report-bad',
            expect.objectContaining({ method: 'POST' })
        )
        const [, opts] = fetchFn.mock.calls[0]
        expect(JSON.parse(opts.body)).toEqual({
            id: 'build-200-ios',
            hash: 'HASH',
            platform: 'ios',
            error: 'rolled back',
        })
    })
})

describe('postBootBeacon', () => {
    it('POSTs the bundle id/platform/hash to /api/app/boot', async () => {
        const fetchFn = vi.fn().mockResolvedValue({ ok: true, status: 200 })
        await postBootBeacon({
            serverUrl: 'https://srv.test',
            platform: 'ios',
            id: 'build-200-ios',
            hash: 'HASH',
            fetchFn,
        })
        expect(fetchFn).toHaveBeenCalledWith('https://srv.test/api/app/boot', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ id: 'build-200-ios', platform: 'ios', hash: 'HASH' }),
        })
    })

    it('throws on a non-ok response', async () => {
        const fetchFn = vi.fn().mockResolvedValue({ ok: false, status: 500 })
        await expect(
            postBootBeacon({
                serverUrl: 'https://srv.test',
                platform: 'ios',
                id: 'x',
                hash: 'h',
                fetchFn,
            })
        ).rejects.toThrow(/500/)
    })
})

describe('isUpdateTransportAllowed', () => {
    // Pass `env` explicitly so these are hermetic — never touch the real
    // process.env (which has no bypass flag in CI anyway).
    const noBypass = {}
    const withBypass = { EXPO_PUBLIC_ALLOW_INSECURE_UPDATES: '1' }

    it('allows https://', () => {
        expect(isUpdateTransportAllowed('https://srv.test', noBypass)).toBe(true)
    })

    it('blocks plaintext http:// without a bypass', () => {
        expect(isUpdateTransportAllowed('http://srv.test', noBypass)).toBe(false)
    })

    it('allows http://localhost (dev testing)', () => {
        expect(isUpdateTransportAllowed('http://localhost:8081', noBypass)).toBe(true)
    })

    it('allows http://127.0.0.1 (dev testing)', () => {
        expect(isUpdateTransportAllowed('http://127.0.0.1:8081', noBypass)).toBe(true)
    })

    it('allows http:// when the bypass env var is set truthy', () => {
        expect(isUpdateTransportAllowed('http://srv.test', withBypass)).toBe(true)
    })

    it('treats falsey bypass values as not set', () => {
        expect(
            isUpdateTransportAllowed('http://srv.test', { EXPO_PUBLIC_ALLOW_INSECURE_UPDATES: '0' })
        ).toBe(false)
        expect(
            isUpdateTransportAllowed('http://srv.test', {
                EXPO_PUBLIC_ALLOW_INSECURE_UPDATES: 'false',
            })
        ).toBe(false)
        expect(
            isUpdateTransportAllowed('http://srv.test', { EXPO_PUBLIC_ALLOW_INSECURE_UPDATES: '' })
        ).toBe(false)
    })

    it('fails closed on an unparseable address', () => {
        expect(isUpdateTransportAllowed('not a url', withBypass)).toBe(false)
    })
})
