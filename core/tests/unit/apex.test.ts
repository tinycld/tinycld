import {
    isOrgUnderApex,
    looksLikeApexResponse,
    orgUrlUnderApex,
    slugUnderApex,
} from '@tinycld/core/lib/apex'
import { describe, expect, it } from 'vitest'

describe('apex', () => {
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
