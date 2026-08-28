import {
    APP_PREFIX,
    activeSlugFromPathname,
    appHref,
    CONNECT_HREF,
    PICK_ORG_HREF,
    useOrgHref,
} from '@tinycld/core/lib/org-routes'
import { describe, expect, it } from 'vitest'

describe('appHref', () => {
    it('prefixes a path', () => {
        expect(appHref('mail')).toBe('/a/mail')
        expect(appHref('settings/personal')).toBe('/a/settings/personal')
    })

    it('maps the empty path to the bare prefix, with no trailing slash', () => {
        // '/a/' would be a distinct path from the route it should match.
        expect(appHref('')).toBe('/a')
    })

    it('agrees with the exported pre-auth constants', () => {
        expect(CONNECT_HREF).toBe(`${APP_PREFIX}/connect`)
        expect(PICK_ORG_HREF).toBe(`${APP_PREFIX}/pick-org`)
    })
})

describe('useOrgHref', () => {
    // Not stateful — calling outside a component is fine and keeps the test flat.
    const orgHref = useOrgHref()

    it('returns a prefixed string when there are no params', () => {
        expect(orgHref('mail')).toBe('/a/mail')
        expect(orgHref('')).toBe('/a')
    })

    it('returns a STRING, not an object, when there are no params', () => {
        // Load-bearing: an object href is a new identity every render, which
        // makes <Redirect> re-navigate forever (React #185).
        expect(typeof orgHref('mail')).toBe('string')
        expect(typeof orgHref('mail', {})).toBe('string')
    })

    it('returns an object with a prefixed pathname when params are present', () => {
        expect(orgHref('mail/[id]', { id: '123' })).toEqual({
            pathname: '/a/mail/[id]',
            params: { id: '123' },
        })
    })
})

describe('activeSlugFromPathname', () => {
    it('reads the slug from the segment after the prefix', () => {
        expect(activeSlugFromPathname('/a/mail')).toBe('mail')
        expect(activeSlugFromPathname('/a/mail/thread-1')).toBe('mail')
        expect(activeSlugFromPathname('/a/drive/folder/f1')).toBe('drive')
    })

    it('ignores a query string', () => {
        expect(activeSlugFromPathname('/a/mail?folder=sent')).toBe('mail')
    })

    it('returns null at the workspace root and outside the prefix', () => {
        expect(activeSlugFromPathname('/a')).toBeNull()
        expect(activeSlugFromPathname('/a/')).toBeNull()
        expect(activeSlugFromPathname('/')).toBeNull()
        expect(activeSlugFromPathname('/p/demo')).toBeNull()
    })
})
