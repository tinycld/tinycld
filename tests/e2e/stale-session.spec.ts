import { expect, test } from '@playwright/test'
import { authStorageKey } from './auth-key-helpers'
import { appShell, login } from './helpers'

// A PocketBase auth token carries a JWT `exp`; when a session outlives it the
// token string is still sitting in storage but every request now fails. The old
// gate keyed "logged in" on token PRESENCE (`!!pb.authStore.token`), so a dead
// token presented as a signed-in-but-broken session: all fetches returned no
// records and any create threw for want of org context. This spec forges an
// expired token into the app's stored auth and asserts the app now recovers to
// the login gate instead of that broken half-session.
test('an expired token recovers to the login gate, not a broken half-session', async ({ page }) => {
    await login(page)
    await expect(appShell(page)).toBeVisible()

    // Read the app's own stored auth, expire its token, write it back — exactly
    // the on-disk state of a session whose token lapsed while the app was closed.
    // The rewrite runs in the browser: it re-base64url's the JWT payload with a
    // past `exp` (isValid reads only the unsigned payload, so this trips expiry),
    // and the mangled signature guarantees the server rejects authRefresh() with
    // 401 — the authoritative-rejection path that clears the session.
    const authKey = await authStorageKey(page)
    const rewritten = await page.evaluate(key => {
        const raw = window.localStorage.getItem(key)
        if (!raw) return { ok: false as const, reason: `no ${key} in storage` }
        const parsed = JSON.parse(raw) as { token: string; record?: unknown; model?: unknown }
        const [header, payload, signature] = parsed.token.split('.')
        const decoded = JSON.parse(atob(payload.replace(/-/g, '+').replace(/_/g, '/'))) as {
            exp: number
            [k: string]: unknown
        }
        decoded.exp = 1000000000 // 2001-09-09 — comfortably in the past
        const nextPayload = btoa(JSON.stringify(decoded))
            .replace(/\+/g, '-')
            .replace(/\//g, '_')
            .replace(/=+$/, '')
        parsed.token = `${header}.${nextPayload}.${signature}`
        window.localStorage.setItem(key, JSON.stringify(parsed))
        return { ok: true as const }
    }, authKey)
    expect(rewritten.ok, rewritten.ok ? '' : rewritten.reason).toBe(true)

    // Reload: boot re-hydrates from the forged storage. The hardened gate sees an
    // invalid token and refreshAuth() clears the dead session, so the app must
    // land on the login gate rather than a signed-in-but-empty workspace.
    await page.goto(`/`)

    await expect(page.getByTestId('identifier')).toBeVisible({ timeout: 20_000 })
    await expect(page.getByText('Sign in', { exact: true }).last()).toBeVisible()

    // And the storage-level session was actually cleared (not just visually
    // gated) — a lingering token would resurrect the broken state on the next nav.
    const cleared = await page.evaluate(key => {
        const raw = window.localStorage.getItem(key)
        if (!raw) return true
        try {
            const { token } = JSON.parse(raw) as { token?: string }
            return !token
        } catch {
            return true
        }
    }, authKey)
    expect(cleared).toBe(true)
})
