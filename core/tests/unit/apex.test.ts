import {
    isApexAddress,
    isOrgUnderApex,
    looksLikeApexResponse,
    orgUrlUnderApex,
    slugUnderApex,
} from '@tinycld/core/lib/apex'
import { afterEach, describe, expect, it, vi } from 'vitest'

describe('apex', () => {
    afterEach(() => {
        vi.unstubAllGlobals()
    })

    describe('isApexAddress', () => {
        function mockFetch(contentType: string, body: string) {
            vi.stubGlobal(
                'fetch',
                vi.fn().mockResolvedValue({
                    headers: { get: () => contentType },
                    text: async () => body,
                })
            )
        }

        // The regression: this used to compare the address against the
        // configured defaultServer, which IS an ordinary single-tenant server's
        // address in the common case. Every such user got "localhost hosts many
        // organizations" and the org picker instead of a login form.
        it('reports a single-tenant server as NOT an apex', async () => {
            mockFetch('application/json', JSON.stringify({ name: 'My Server' }))
            await expect(isApexAddress('http://localhost:7100')).resolves.toBe(false)
        })

        it('reports the router apex as an apex', async () => {
            mockFetch('text/html; charset=utf-8', '<!doctype html><html lang="en">')
            await expect(isApexAddress('https://tinycld.org')).resolves.toBe(true)
        })

        it('is false for no address, and for an unreachable one', async () => {
            await expect(isApexAddress(null)).resolves.toBe(false)
            vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('offline')))
            await expect(isApexAddress('https://down.example')).resolves.toBe(false)
        })
    })

    describe('looksLikeApexResponse', () => {
        it('detects the router org-finder page by content type', () => {
            expect(looksLikeApexResponse('text/html; charset=utf-8', '<!doctype html>')).toBe(true)
        })

        it('sniffs a doctype when the content type is unhelpful', () => {
            expect(looksLikeApexResponse('text/plain', '\n  <!DOCTYPE html><html>')).toBe(true)
            expect(looksLikeApexResponse('', '<html lang="en">')).toBe(true)
        })

        it('leaves real org-info JSON alone', () => {
            expect(looksLikeApexResponse('application/json', '{"name":"Acme"}')).toBe(false)
        })

        // An org could legitimately be named "<html>"; the body must not be
        // sniffed anywhere but at its very start.
        it('does not flag JSON that merely contains markup', () => {
            expect(looksLikeApexResponse('application/json', '{"name":"<html> Ltd"}')).toBe(false)
        })
    })

    describe('orgUrlUnderApex', () => {
        it('builds an org origin as a child of the apex', () => {
            expect(orgUrlUnderApex('acme', 'tinycld.org')).toBe('https://acme.tinycld.org')
        })

        // The slug becomes the leftmost label of a URL we navigate to, so the
        // DNS-label rule is a security boundary, not tidiness.
        it('rejects anything that is not a single DNS label', () => {
            expect(orgUrlUnderApex('a.b', 'tinycld.org')).toBeNull()
            expect(orgUrlUnderApex('evil.example/x', 'tinycld.org')).toBeNull()
            expect(orgUrlUnderApex('UPPER', 'tinycld.org')).toBeNull()
            expect(orgUrlUnderApex('', 'tinycld.org')).toBeNull()
        })
    })

    describe('isOrgUnderApex / slugUnderApex', () => {
        it('accepts a direct child of the apex', () => {
            expect(isOrgUnderApex('https://acme.tinycld.org', 'tinycld.org')).toBe(true)
            expect(slugUnderApex('https://acme.tinycld.org', 'tinycld.org')).toBe('acme')
        })

        it('rejects the apex itself', () => {
            expect(isOrgUnderApex('https://tinycld.org', 'tinycld.org')).toBe(false)
        })

        it('rejects unrelated and deeper hosts', () => {
            expect(isOrgUnderApex('https://pb.example.com', 'tinycld.org')).toBe(false)
            expect(isOrgUnderApex('https://a.b.tinycld.org', 'tinycld.org')).toBe(false)
            // A lookalike registrable domain must not pass as a child.
            expect(isOrgUnderApex('https://eviltinycld.org', 'tinycld.org')).toBe(false)
        })

        it('returns null for a slug of a non-org origin', () => {
            expect(slugUnderApex('https://pb.example.com', 'tinycld.org')).toBeNull()
        })
    })
})
