import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const TEST_CONFIG = {
    brandName: 'TestApp',
    serverShortcuts: {},
} as const

async function importFresh() {
    vi.resetModules()
    const config = await import('@tinycld/core/lib/core-config')
    config.configureCore(TEST_CONFIG)
    return await import('@tinycld/core/lib/server-address')
}

async function importFreshNoConfig() {
    vi.resetModules()
    return await import('@tinycld/core/lib/server-address')
}

describe('server-address', () => {
    beforeEach(() => {
        delete process.env.EXPO_PUBLIC_ENV
    })

    afterEach(() => {
        vi.unstubAllGlobals()
        vi.restoreAllMocks()
    })

    describe('resolveEnvAddress', () => {
        // On web (Platform.OS === 'web' in unit-setup.ts) we ignore
        // EXPO_PUBLIC_ENV and serverShortcuts entirely — every web build is
        // same-origin, served by the dev proxy or production app server.

        it('returns the window origin on web regardless of EXPO_PUBLIC_ENV', async () => {
            vi.stubGlobal('window', { location: { origin: 'https://example.com' } })
            const { resolveEnvAddress } = await importFresh()
            expect(resolveEnvAddress()).toBe('https://example.com')
        })

        it('uses webShortcut when provided', async () => {
            vi.stubGlobal('window', { location: { origin: 'https://fallback.example' } })
            vi.resetModules()
            const config = await import('@tinycld/core/lib/core-config')
            config.configureCore({
                brandName: 'Acme',
                serverShortcuts: {},
                webShortcut: () => 'https://web.acme.example',
            })
            const { resolveEnvAddress } = await import('@tinycld/core/lib/server-address')
            expect(resolveEnvAddress()).toBe('https://web.acme.example')
        })

        it('returns null when no config has been registered', async () => {
            const { resolveEnvAddress } = await importFreshNoConfig()
            expect(resolveEnvAddress()).toBeNull()
        })
    })

    describe('normalizeAddress', () => {
        it('prepends https:// when scheme is missing', async () => {
            const { normalizeAddress } = await importFresh()
            expect(normalizeAddress('pb.example.com')).toBe('https://pb.example.com')
        })

        it('preserves http:// scheme', async () => {
            const { normalizeAddress } = await importFresh()
            expect(normalizeAddress('http://localhost:8090')).toBe('http://localhost:8090')
        })

        it('strips trailing slashes', async () => {
            const { normalizeAddress } = await importFresh()
            expect(normalizeAddress('https://pb.example.com/')).toBe('https://pb.example.com')
            expect(normalizeAddress('https://pb.example.com///')).toBe('https://pb.example.com')
        })

        it('trims whitespace', async () => {
            const { normalizeAddress } = await importFresh()
            expect(normalizeAddress('  https://pb.example.com  ')).toBe('https://pb.example.com')
        })
    })

    describe('readCached / writeCached / clearCached', () => {
        it('roundtrips an address', async () => {
            vi.stubGlobal('window', { location: { origin: 'https://a.example' } })
            const { readCached, writeCached, clearCached } = await importFresh()
            expect(await readCached()).toBeNull()
            await writeCached('https://pb.example.com')
            expect(await readCached()).toBe('https://pb.example.com')
            await clearCached()
            expect(await readCached()).toBeNull()
        })

        it('uses origin-specific keys on web', async () => {
            vi.stubGlobal('window', { location: { origin: 'https://a.example' } })
            const modA = await importFresh()
            await modA.writeCached('https://pb-a.example.com')

            vi.stubGlobal('window', { location: { origin: 'https://b.example' } })
            const modB = await importFresh()
            expect(await modB.readCached()).toBeNull()

            await modB.writeCached('https://pb-b.example.com')
            expect(await modB.readCached()).toBe('https://pb-b.example.com')

            vi.stubGlobal('window', { location: { origin: 'https://a.example' } })
            const modA2 = await importFresh()
            expect(await modA2.readCached()).toBe('https://pb-a.example.com')
        })
    })

    describe('probe', () => {
        it('resolves when /api/health returns 200', async () => {
            const fetchMock = vi.fn().mockResolvedValue({ ok: true, status: 200 })
            vi.stubGlobal('fetch', fetchMock)
            const { probe } = await importFresh()
            await expect(probe('https://pb.example.com')).resolves.toBeUndefined()
            expect(fetchMock).toHaveBeenCalledWith(
                'https://pb.example.com/api/health',
                expect.objectContaining({ signal: expect.anything() })
            )
        })

        it('rejects on non-2xx', async () => {
            vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false, status: 404 }))
            const { probe } = await importFresh()
            await expect(probe('https://pb.example.com')).rejects.toThrow('HTTP 404')
        })

        it('rejects on network error', async () => {
            vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('Failed to fetch')))
            const { probe } = await importFresh()
            await expect(probe('https://pb.example.com')).rejects.toThrow('Failed to fetch')
        })
    })

    describe('probeServer', () => {
        // The apex's real response, captured from the router: it serves the
        // org-finder page with HTTP 200 for EVERY path, /api/* included.
        const APEX_HTML = '<!doctype html>\n<html lang="en">\n<head>\n<meta charset="utf-8">'

        function mockResponse(init: {
            ok?: boolean
            status?: number
            contentType?: string
            body: string
        }) {
            return {
                ok: init.ok ?? true,
                status: init.status ?? 200,
                headers: { get: () => init.contentType ?? 'application/json' },
                text: async () => init.body,
            }
        }

        it('resolves against a real server answering /api/org-info', async () => {
            const fetchMock = vi
                .fn()
                .mockResolvedValue(mockResponse({ body: JSON.stringify({ name: 'Acme' }) }))
            vi.stubGlobal('fetch', fetchMock)
            const { probeServer } = await importFresh()
            await expect(probeServer('https://acme.tinycld.org')).resolves.toBeUndefined()
            expect(fetchMock).toHaveBeenCalledWith(
                'https://acme.tinycld.org/api/org-info',
                expect.objectContaining({ signal: expect.anything() })
            )
        })

        it('accepts a server whose app name is empty', async () => {
            vi.stubGlobal(
                'fetch',
                vi.fn().mockResolvedValue(mockResponse({ body: JSON.stringify({ name: '' }) }))
            )
            const { probeServer } = await importFresh()
            await expect(probeServer('https://pb.example.com')).resolves.toBeUndefined()
        })

        // The regression this whole change exists for: a status-only check
        // admitted the apex, and the app then rendered a sign-in panel against
        // a host with no PocketBase behind it.
        it('rejects the hosting apex despite its HTTP 200', async () => {
            vi.stubGlobal(
                'fetch',
                vi
                    .fn()
                    .mockResolvedValue(
                        mockResponse({ contentType: 'text/html; charset=utf-8', body: APEX_HTML })
                    )
            )
            const { probeServer } = await importFresh()
            const { ApexServerError } = await import('@tinycld/core/lib/apex')
            await expect(probeServer('https://tinycld.org')).rejects.toBeInstanceOf(ApexServerError)
        })

        it('detects an apex that serves HTML without an html content type', async () => {
            vi.stubGlobal(
                'fetch',
                vi
                    .fn()
                    .mockResolvedValue(mockResponse({ contentType: 'text/plain', body: APEX_HTML }))
            )
            const { probeServer } = await importFresh()
            const { ApexServerError } = await import('@tinycld/core/lib/apex')
            await expect(probeServer('https://tinycld.org')).rejects.toBeInstanceOf(ApexServerError)
        })

        it('rejects JSON that is not org-info shaped', async () => {
            vi.stubGlobal(
                'fetch',
                vi.fn().mockResolvedValue(mockResponse({ body: JSON.stringify({ status: 'ok' }) }))
            )
            const { probeServer } = await importFresh()
            await expect(probeServer('https://not-tinycld.example')).rejects.toThrow(
                'did not answer like a TinyCld server'
            )
        })

        it('rejects a non-2xx JSON response', async () => {
            vi.stubGlobal(
                'fetch',
                vi
                    .fn()
                    .mockResolvedValue(
                        mockResponse({ ok: false, status: 502, body: '{"message":"bad gateway"}' })
                    )
            )
            const { probeServer } = await importFresh()
            await expect(probeServer('https://pb.example.com')).rejects.toThrow('HTTP 502')
        })

        it('rejects on network error', async () => {
            vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('Failed to fetch')))
            const { probeServer } = await importFresh()
            await expect(probeServer('https://pb.example.com')).rejects.toThrow('Failed to fetch')
        })
    })

    describe('setResolvedAddress / getResolvedAddress', () => {
        it('roundtrips', async () => {
            vi.stubGlobal('window', { location: { origin: 'https://example.com' } })
            const { setResolvedAddress, getResolvedAddress } = await importFresh()
            // module-load auto-resolution sets the web origin; override it
            setResolvedAddress('https://pb.example.com')
            expect(getResolvedAddress()).toBe('https://pb.example.com')
        })

        it('auto-sets to window origin on module load when config is registered', async () => {
            vi.stubGlobal('window', { location: { origin: 'https://auto.example' } })
            const { getResolvedAddress } = await importFresh()
            expect(getResolvedAddress()).toBe('https://auto.example')
        })
    })
})
