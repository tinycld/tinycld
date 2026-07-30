// @vitest-environment happy-dom
import { describe, expect, it } from 'vitest'

import { ORGS_COOKIE_NAME, orgUrlForSlug, parseOrgsCookie } from '../../lib/org-cookie'

function cookieFor(value: unknown): string {
    return `${ORGS_COOKIE_NAME}=${encodeURIComponent(JSON.stringify(value))}`
}

describe('parseOrgsCookie', () => {
    it('parses valid entries out of a multi-cookie header', () => {
        const header = `theme=dark; ${cookieFor([
            { slug: 'acme', name: 'Acme Inc' },
            { slug: 'beta', name: 'Beta LLC' },
        ])}; other=1`
        expect(parseOrgsCookie(header)).toEqual([
            { slug: 'acme', name: 'Acme Inc' },
            { slug: 'beta', name: 'Beta LLC' },
        ])
    })

    it('drops malformed entries and falls back to slug for a missing name', () => {
        const header = cookieFor([
            { slug: 'acme', name: '' },
            { slug: '', name: 'No slug' },
            'not-an-object',
        ])
        expect(parseOrgsCookie(header)).toEqual([{ slug: 'acme', name: 'acme' }])
    })

    // The cookie is writable by JS on any sibling tenant, and the slug becomes
    // the leftmost label of a URL this app will navigate to. Anything that is
    // not a single lowercase DNS label is a planted entry.
    it('drops entries whose slug is not a single DNS label', () => {
        const header = cookieFor([
            { slug: 'evil.example/x', name: 'Path smuggle' },
            { slug: 'a.b', name: 'Dotted' },
            { slug: 'UPPER', name: 'Shouty' },
            { slug: '-lead', name: 'Bad hyphen' },
            { slug: 'good-org', name: 'Fine' },
        ])
        expect(parseOrgsCookie(header)).toEqual([{ slug: 'good-org', name: 'Fine' }])
    })

    // Cookies written before the url field was dropped still parse — the
    // stored url is ignored, never surfaced.
    it('ignores the legacy url field', () => {
        const header = cookieFor([
            { slug: 'acme', name: 'Acme', url: 'https://evil.example/login' },
        ])
        expect(parseOrgsCookie(header)).toEqual([{ slug: 'acme', name: 'Acme' }])
    })

    it('degrades to empty on absent or unparsable cookies', () => {
        expect(parseOrgsCookie('')).toEqual([])
        expect(parseOrgsCookie('theme=dark')).toEqual([])
        expect(parseOrgsCookie(`${ORGS_COOKIE_NAME}=%%%not-json`)).toEqual([])
        expect(parseOrgsCookie(cookieFor({ not: 'an array' }))).toEqual([])
    })
})

describe('orgUrlForSlug', () => {
    it('derives the sibling origin from the current hostname', () => {
        expect(orgUrlForSlug('beta', 'acme.tinycld.org')).toBe('https://beta.tinycld.org')
        expect(orgUrlForSlug('beta', 'acme.tenants.example.test')).toBe(
            'https://beta.tenants.example.test'
        )
    })

    it('refuses when no parent domain exists or the slug is not a label', () => {
        expect(orgUrlForSlug('beta', 'localhost')).toBeNull()
        expect(orgUrlForSlug('evil.example/x', 'acme.tinycld.org')).toBeNull()
        expect(orgUrlForSlug('', 'acme.tinycld.org')).toBeNull()
    })
})
