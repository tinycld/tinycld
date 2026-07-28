// @vitest-environment happy-dom
import { describe, expect, it } from 'vitest'

import { ORGS_COOKIE_NAME, parseOrgsCookie } from '../../lib/org-cookie'

const entry = (slug: string, name: string, url: string) => ({ slug, name, url })

function cookieFor(value: unknown): string {
    return `${ORGS_COOKIE_NAME}=${encodeURIComponent(JSON.stringify(value))}`
}

describe('parseOrgsCookie', () => {
    it('parses valid entries out of a multi-cookie header', () => {
        const header = `theme=dark; ${cookieFor([
            entry('acme', 'Acme Inc', 'https://acme.tinycld.org'),
            entry('beta', 'Beta LLC', 'https://beta.tinycld.org'),
        ])}; other=1`
        expect(parseOrgsCookie(header)).toEqual([
            { slug: 'acme', name: 'Acme Inc', url: 'https://acme.tinycld.org' },
            { slug: 'beta', name: 'Beta LLC', url: 'https://beta.tinycld.org' },
        ])
    })

    it('drops malformed entries and falls back to slug for a missing name', () => {
        const header = cookieFor([
            entry('acme', '', 'https://acme.tinycld.org'),
            { slug: '', name: 'No slug', url: 'https://x.tinycld.org' },
            { slug: 'evil', name: 'Evil', url: 'javascript:alert(1)' },
            'not-an-object',
        ])
        expect(parseOrgsCookie(header)).toEqual([
            { slug: 'acme', name: 'acme', url: 'https://acme.tinycld.org' },
        ])
    })

    it('degrades to empty on absent or unparsable cookies', () => {
        expect(parseOrgsCookie('')).toEqual([])
        expect(parseOrgsCookie('theme=dark')).toEqual([])
        expect(parseOrgsCookie(`${ORGS_COOKIE_NAME}=%%%not-json`)).toEqual([])
        expect(parseOrgsCookie(cookieFor({ not: 'an array' }))).toEqual([])
    })
})
