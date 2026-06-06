import { setResolvedAddress } from '@tinycld/core/lib/server-address'
import { fetchReleaseManifest } from '@tinycld/core/lib/use-release-manifest'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

function mockFetch(impl: () => unknown) {
    globalThis.fetch = vi.fn(async () => impl()) as unknown as typeof fetch
}

beforeEach(() => {
    setResolvedAddress('https://pb.example.com')
})

afterEach(() => {
    setResolvedAddress(null)
    vi.restoreAllMocks()
})

describe('fetchReleaseManifest', () => {
    it('returns the parsed manifest on a 200', async () => {
        mockFetch(() => ({
            ok: true,
            json: async () => ({
                appTag: 'v0.0.3',
                releasedAt: '2026-06-05T12:00:00.000Z',
                members: [{ name: 'mail', repo: 'tinycld/mail', tag: 'v0.1.0', sha: 'abc1234' }],
            }),
        }))

        const manifest = await fetchReleaseManifest()
        expect(manifest.appTag).toBe('v0.0.3')
        expect(manifest.members).toHaveLength(1)
        expect(manifest.members[0]?.name).toBe('mail')
    })

    it('returns an empty members list on a non-ok response', async () => {
        mockFetch(() => ({ ok: false, json: async () => ({ members: [{ name: 'mail' }] }) }))
        const manifest = await fetchReleaseManifest()
        expect(manifest.members).toEqual([])
    })

    it('normalizes a missing members field to an empty array', async () => {
        mockFetch(() => ({ ok: true, json: async () => ({ appTag: 'v0.0.3' }) }))
        const manifest = await fetchReleaseManifest()
        expect(manifest.appTag).toBe('v0.0.3')
        expect(manifest.members).toEqual([])
    })
})
