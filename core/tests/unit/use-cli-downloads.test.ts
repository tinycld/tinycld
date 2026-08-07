import { setResolvedAddress } from '@tinycld/core/lib/server-address'
import {
    type CliDownload,
    detectCliOS,
    downloadLabel,
    fetchCliDownloads,
    orderForViewer,
} from '@tinycld/core/lib/use-cli-downloads'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const entry = (os: string, arch: string): CliDownload => ({
    platform: `${os}-${arch}`,
    os,
    arch,
    filename: os === 'windows' ? 'tinycld.exe' : 'tinycld',
    size: 100,
    url: `/api/cli/download/${os}-${arch}`,
})

const all = [
    entry('darwin', 'arm64'),
    entry('darwin', 'amd64'),
    entry('linux', 'amd64'),
    entry('linux', 'arm64'),
    entry('windows', 'amd64'),
]

describe('downloadLabel', () => {
    it('labels every platform, distinguishing mac arches', () => {
        expect(all.map(downloadLabel)).toEqual([
            'macOS (Apple Silicon)',
            'macOS (Intel)',
            'Linux (x64)',
            'Linux (arm64)',
            'Windows (x64)',
        ])
    })
})

describe('orderForViewer', () => {
    it('puts the detected OS first, keeping server order otherwise', () => {
        const ordered = orderForViewer(all, 'windows')
        expect(ordered.map(d => d.platform)).toEqual([
            'windows-amd64',
            'darwin-arm64',
            'darwin-amd64',
            'linux-amd64',
            'linux-arm64',
        ])
    })

    it('is a no-op with no detection', () => {
        expect(orderForViewer(all, null)).toEqual(all)
    })
})

describe('detectCliOS', () => {
    // Platform.OS is 'web' under the unit-test setup (react-native-web).
    it('maps user agents to CLI target OSes', () => {
        expect(
            detectCliOS('Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36')
        ).toBe('darwin')
        expect(detectCliOS('Mozilla/5.0 (Windows NT 10.0; Win64; x64)')).toBe('windows')
        expect(detectCliOS('Mozilla/5.0 (X11; Linux x86_64)')).toBe('linux')
        expect(detectCliOS('SomethingElse/1.0')).toBeNull()
    })
})

describe('fetchCliDownloads', () => {
    beforeEach(() => {
        setResolvedAddress('https://pb.example.com')
    })

    afterEach(() => {
        vi.unstubAllGlobals()
        setResolvedAddress(null)
    })

    it('returns the listed downloads', async () => {
        vi.stubGlobal(
            'fetch',
            vi.fn(async () => ({
                ok: true,
                json: async () => ({ downloads: [entry('darwin', 'arm64')] }),
            }))
        )
        const got = await fetchCliDownloads()
        expect(got).toHaveLength(1)
        expect(got[0].platform).toBe('darwin-arm64')
    })

    it('returns [] on a non-OK response', async () => {
        vi.stubGlobal(
            'fetch',
            vi.fn(async () => ({ ok: false, json: async () => ({}) }))
        )
        expect(await fetchCliDownloads()).toEqual([])
    })

    it('returns [] when the body has no downloads key', async () => {
        vi.stubGlobal(
            'fetch',
            vi.fn(async () => ({ ok: true, json: async () => ({}) }))
        )
        expect(await fetchCliDownloads()).toEqual([])
    })
})
