import { CONNECT_HREF } from '@tinycld/core/lib/org-routes'
import {
    clearPendingRoute,
    setPendingRoute,
    takePendingRoute,
} from '@tinycld/core/lib/pending-route'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

describe('pending-route', () => {
    beforeEach(() => {
        clearPendingRoute()
    })

    it('returns the recorded route and clears it, so a later sign-in starts clean', () => {
        setPendingRoute('/a/settings')
        expect(takePendingRoute()).toBe('/a/settings')
        expect(takePendingRoute()).toBeNull()
    })

    it('preserves the query string, which encodes package view state', () => {
        setPendingRoute('/a/boards?focused=HOME-1')
        expect(takePendingRoute()).toBe('/a/boards?focused=HOME-1')
    })

    it('keeps the OAuth consent code so the device flow can resume', () => {
        setPendingRoute('/p/oauth/authorize?user_code=ABCD-1234')
        expect(takePendingRoute()).toBe('/p/oauth/authorize?user_code=ABCD-1234')
    })

    it('clears a recorded route', () => {
        setPendingRoute('/a/settings')
        clearPendingRoute()
        expect(takePendingRoute()).toBeNull()
    })

    it('returns null when nothing was recorded', () => {
        expect(takePendingRoute()).toBeNull()
    })

    // Landing routes: restoring these just re-enters the redirect that the
    // no-pending fallback already performs.
    it.each(['/', '/a', CONNECT_HREF, '/p/demo'])('ignores the landing/pre-auth route %s', href => {
        setPendingRoute(href)
        expect(takePendingRoute()).toBeNull()
    })

    it('ignores the root even when it carries a query string', () => {
        setPendingRoute('/?foo=1')
        expect(takePendingRoute()).toBeNull()
    })

    // Open-redirect guard. '//evil.example.com' is the one the existing backTo
    // consumers miss: a browser reads it as an absolute off-site URL.
    it.each([
        'https://evil.example.com',
        '//evil.example.com',
        'evil.example.com',
        'javascript:alert(1)',
    ])('refuses the off-site href %s', href => {
        setPendingRoute(href)
        expect(takePendingRoute()).toBeNull()
    })

    it('does not overwrite a good route with a rejected one', () => {
        setPendingRoute('/a/settings')
        setPendingRoute('//evil.example.com')
        expect(takePendingRoute()).toBe('/a/settings')
    })
})

// The web entry URL is snapshotted at import time, because a deep-linked
// signed-out load collapses the address bar to the bare app root before the
// sign-in form renders (see pending-route.ts). These re-import the module with
// a stubbed location to exercise that boot-time capture.
describe('pending-route entry-URL capture', () => {
    afterEach(() => {
        vi.unstubAllGlobals()
        vi.resetModules()
    })

    async function importWithLocation(pathname: string, search = '') {
        vi.stubGlobal('location', { pathname, search })
        vi.resetModules()
        return await import('@tinycld/core/lib/pending-route')
    }

    it('captures a deep entry URL at import time', async () => {
        const mod = await importWithLocation('/a/settings/personal')
        expect(mod.takePendingRoute()).toBe('/a/settings/personal')
    })

    it('keeps the query string of the entry URL', async () => {
        const mod = await importWithLocation('/a/boards', '?focused=HOME-1')
        expect(mod.takePendingRoute()).toBe('/a/boards?focused=HOME-1')
    })

    it('captures nothing when the app is opened at the root', async () => {
        const mod = await importWithLocation('/')
        expect(mod.takePendingRoute()).toBeNull()
    })

    it('captures nothing when the app root is what loaded', async () => {
        const mod = await importWithLocation('/a')
        expect(mod.takePendingRoute()).toBeNull()
    })
})
