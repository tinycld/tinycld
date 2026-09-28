import {
    APP_PREFIX,
    activeSlugFromPathname,
    appHref,
    CONNECT_HREF,
    normalizeLegacyAppPath,
    useOrgHref,
} from '@tinycld/core/lib/org-routes'
import { describe, expect, it } from 'vitest'

describe('appHref', () => {
    it('prefixes a path', () => {
        expect(appHref('gizmos')).toBe('/a/gizmos')
        expect(appHref('settings/personal')).toBe('/a/settings/personal')
    })

    it('maps the empty path to the bare prefix, with no trailing slash', () => {
        // '/a/' would be a distinct path from the route it should match.
        expect(appHref('')).toBe('/a')
    })

    it('agrees with the exported pre-auth constant', () => {
        expect(CONNECT_HREF).toBe(`${APP_PREFIX}/connect`)
    })
})

describe('useOrgHref', () => {
    // Not stateful — calling outside a component is fine and keeps the test flat.
    const orgHref = useOrgHref()

    it('returns a prefixed string when there are no params', () => {
        expect(orgHref('gizmos')).toBe('/a/gizmos')
        expect(orgHref('')).toBe('/a')
    })

    it('returns a STRING, not an object, when there are no params', () => {
        // Load-bearing: an object href is a new identity every render, which
        // makes <Redirect> re-navigate forever (React #185).
        expect(typeof orgHref('gizmos')).toBe('string')
        expect(typeof orgHref('gizmos', {})).toBe('string')
    })

    it('returns an object with a prefixed pathname when params are present', () => {
        expect(orgHref('gizmos/[id]', { id: '123' })).toEqual({
            pathname: '/a/gizmos/[id]',
            params: { id: '123' },
        })
    })
})

describe('normalizeLegacyAppPath', () => {
    it('prefixes a legacy app path, preserving the query', () => {
        expect(normalizeLegacyAppPath('/accept-invite/tok')).toBe('/a/accept-invite/tok')
        expect(normalizeLegacyAppPath('/settings/personal')).toBe('/a/settings/personal')
        expect(normalizeLegacyAppPath('/settings?tab=account')).toBe('/a/settings?tab=account')
    })

    it('is idempotent for already-prefixed paths', () => {
        expect(normalizeLegacyAppPath('/a/settings')).toBe('/a/settings')
        expect(normalizeLegacyAppPath('/a')).toBe('/a')
    })

    it('leaves non-app paths untouched', () => {
        // A deep link can legitimately point outside the app tree.
        expect(normalizeLegacyAppPath('/p/cogs/share/tok')).toBe('/p/cogs/share/tok')
        expect(normalizeLegacyAppPath('/api/health')).toBe('/api/health')
        expect(normalizeLegacyAppPath('/dav/cogs/')).toBe('/dav/cogs/')
        expect(normalizeLegacyAppPath('https://example.com/settings')).toBe(
            'https://example.com/settings'
        )
    })

    it('matches whole segments only', () => {
        expect(normalizeLegacyAppPath('/settingsomething')).toBe('/settingsomething')
    })
})

describe('activeSlugFromPathname', () => {
    it('reads the slug from the segment after the prefix', () => {
        expect(activeSlugFromPathname('/a/gizmos')).toBe('gizmos')
        expect(activeSlugFromPathname('/a/gizmos/thread-1')).toBe('gizmos')
        expect(activeSlugFromPathname('/a/cogs/folder/f1')).toBe('cogs')
    })

    it('ignores a query string', () => {
        expect(activeSlugFromPathname('/a/gizmos?folder=sent')).toBe('gizmos')
    })

    it('returns null at the workspace root and outside the prefix', () => {
        expect(activeSlugFromPathname('/a')).toBeNull()
        expect(activeSlugFromPathname('/a/')).toBeNull()
        expect(activeSlugFromPathname('/')).toBeNull()
        expect(activeSlugFromPathname('/p/demo')).toBeNull()
    })
})

/**
 * The hook returns the SAME function every time.
 *
 * Load-bearing, not a micro-optimization: an unstable `orgHref` cannot go in an
 * effect's dependency array, because a new identity per render re-runs the
 * effect every render — and an effect that navigates then re-runs on the render
 * its own navigation caused. React kills that as error #185 (infinite
 * setState), which is what every gadgets deep link crashed with.
 */
describe('useOrgHref identity', () => {
    it('returns a referentially stable builder', () => {
        expect(useOrgHref()).toBe(useOrgHref())
    })

    it('still builds the same hrefs', () => {
        const orgHref = useOrgHref()
        expect(orgHref('gadgets')).toBe('/a/gadgets')
        expect(orgHref('gadgets', { focused: 'PL-1' })).toEqual({
            pathname: '/a/gadgets',
            params: { focused: 'PL-1' },
        })
    })

    // A bare path stays a STRING, which is its own stability guarantee at the
    // call site — <Redirect href={...}> with a fresh object re-navigates every
    // render.
    it('returns a plain string when there are no params', () => {
        expect(typeof useOrgHref()('gadgets/PL-1')).toBe('string')
    })
})
