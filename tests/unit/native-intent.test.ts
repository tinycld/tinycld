import { describe, expect, it } from 'vitest'
import { redirectSystemPath } from '../../app/+native-intent'

// Regression guard for the "Unmatched Route — tinycld:///" boot failure: iOS
// hands a normal home-screen cold launch the app's own custom-scheme URL
// (`tinycld:///`), which must normalize to `/` (→ app/index.tsx). The earlier
// implementation only normalized `http`-prefixed URLs, so the scheme URL fell
// through verbatim and expo-router rendered the Unmatched screen on every launch.
describe('redirectSystemPath', () => {
    it('normalizes the app custom-scheme cold-launch URL to the root route', () => {
        expect(redirectSystemPath({ path: 'tinycld:///', initial: true })).toBe('/')
        expect(redirectSystemPath({ path: 'tinycld://', initial: true })).toBe('/')
    })

    it('rewrites the public /demo contract to the in-app /p/demo route', () => {
        expect(redirectSystemPath({ path: '/demo', initial: false })).toBe('/p/demo')
        expect(redirectSystemPath({ path: 'https://tinycld.org/demo', initial: true })).toBe(
            '/p/demo'
        )
        expect(redirectSystemPath({ path: 'tinycld:///demo', initial: true })).toBe('/p/demo')
        expect(redirectSystemPath({ path: 'https://tinycld.org/demo/foo', initial: true })).toBe(
            '/p/demo'
        )
    })

    it('rewrites legacy unprefixed app links to the current prefix', () => {
        // Links minted before app routes moved under /a — notably emailed
        // invite and reset links opened on a phone — must still resolve.
        expect(redirectSystemPath({ path: '/connect', initial: false })).toBe('/a/connect')
        expect(
            redirectSystemPath({ path: 'https://tinycld.org/accept-invite/tok', initial: true })
        ).toBe('/a/accept-invite/tok')
    })

    it('preserves the query string when rewriting a universal link', () => {
        expect(
            redirectSystemPath({ path: 'https://tinycld.org/settings?tab=account', initial: true })
        ).toBe('/a/settings?tab=account')
    })

    it('leaves already-prefixed and non-app paths alone', () => {
        expect(redirectSystemPath({ path: '/a/mail', initial: false })).toBe('/a/mail')
        expect(redirectSystemPath({ path: '/p/demo', initial: false })).toBe('/p/demo')
    })

    it('normalizes a universal-link root to /', () => {
        expect(redirectSystemPath({ path: 'https://tinycld.org/', initial: true })).toBe('/')
    })
})
