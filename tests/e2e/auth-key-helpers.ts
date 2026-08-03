import type { Page } from '@playwright/test'

// The app stores its PocketBase auth blob under a key scoped to the server:
// `pb_auth:<serverKey>` (see core/lib/auth-storage.ts). Specs that read or stage
// that blob must therefore derive the same key rather than using a literal.
//
// The key is a hash, so it can't be written by hand — but it is also pure,
// synchronous JS over the page's own origin, so we compute it IN THE PAGE using
// the same FNV-1a construction as core/lib/app-updater/server-key.ts. Duplicated
// deliberately: importing app source into a Playwright page context would drag
// in React Native + Metro resolution, and this is 15 lines that a mismatch
// fails loudly on (the specs that use it assert on a token being present).
//
// Web is single-origin, so the key is stable for the whole run.
const AUTH_KEY_IN_PAGE = `(() => {
    const FNV_OFFSET = 0x811c9dc5
    const FNV_PRIME = 0x01000193
    const fnv1a = (input, seed) => {
        let h = FNV_OFFSET ^ seed
        for (let i = 0; i < input.length; i++) {
            h ^= input.charCodeAt(i)
            h = Math.imul(h, FNV_PRIME)
        }
        return h >>> 0
    }
    const origin = new URL(window.location.origin).origin.toLowerCase()
    let hex = ''
    for (let round = 0; round < 4; round++) {
        hex += fnv1a(origin, round).toString(16).padStart(8, '0')
    }
    return 'pb_auth:' + hex
})()`

// authStorageKey returns the key this page's app instance stores auth under.
export async function authStorageKey(page: Page): Promise<string> {
    return page.evaluate(AUTH_KEY_IN_PAGE) as Promise<string>
}

// authKeyInitScript returns a script that defines `window.__PB_AUTH_KEY__`
// before any app code runs, for specs that must STAGE auth via addInitScript
// (where no round-trip to compute the key is possible first).
export function authKeyInitScript(): string {
    return `window.__PB_AUTH_KEY__ = ${AUTH_KEY_IN_PAGE}`
}
