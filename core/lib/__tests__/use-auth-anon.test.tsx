// @vitest-environment happy-dom
//
// `useAuth()` throws for an anonymous viewer unless the caller opts out with
// `throwIfAnon: false`. That default is load-bearing security, not ergonomics:
// callers across the app treat a bare `useAuth()` as proof of an authenticated
// identity and go straight on to run user-scoped queries with `user.id`. The
// mention search is the clearest case — it leaves itself ENABLED on the path
// with no editor mount, and the only thing stopping a signed-out or share
// viewer from enumerating the roster there is that `useAuth()` never returns to
// them at all.
//
// A package test cannot assert this: it mocks `@tinycld/core/lib/auth`, so
// making the mock throw and asserting it throws proves nothing about the real
// hook. It belongs here, against the real implementation.
import { renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const h = vi.hoisted(() => ({
    authStoreState: {
        user: null as { id: string; name: string; email: string; isDemo: boolean } | null,
        hasHydrated: true,
        login: () => {},
        logout: () => {},
    },
    pbAuthStore: { isValid: false },
}))

vi.mock('@tinycld/core/lib/stores/auth-store', () => ({
    useAuthStore: (selector: (s: typeof h.authStoreState) => unknown) => selector(h.authStoreState),
}))

vi.mock('@tinycld/core/lib/pocketbase', () => ({
    pb: { authStore: h.pbAuthStore },
    refreshAuth: vi.fn(),
}))

import { AuthRequiredError, useAuth } from '@tinycld/core/lib/auth'

const USER = { id: 'u1', name: 'Me', email: 'me@example.com', isDemo: false }

function setAnon() {
    h.authStoreState.user = null
    h.pbAuthStore.isValid = false
}

function setAuthed() {
    h.authStoreState.user = USER
    h.pbAuthStore.isValid = true
}

beforeEach(setAnon)

describe('useAuth anonymous default', () => {
    it('throws AuthRequiredError for an anonymous viewer when called with no options', () => {
        expect(() => renderHook(() => useAuth())).toThrow(AuthRequiredError)
    })

    it('throws rather than answering a null user, so a caller cannot read user.id as blank', () => {
        // The failure mode this default prevents: returning `{ user: null }`
        // (or a blank-id user) lets a caller run a user-scoped query whose FK
        // filter matches rows with an empty FK instead of matching nothing.
        expect(() => renderHook(() => useAuth())).toThrow(/Authentication required/)
    })

    it('answers a null user instead of throwing only when the caller opts out', () => {
        const { result } = renderHook(() => useAuth({ throwIfAnon: false }))
        expect(result.current.isLoggedIn).toBe(false)
        expect(result.current.user).toBeNull()
    })

    it('treats a user with an invalid token as anonymous', () => {
        // Expiry does not clear the stored user, so token validity — not the
        // presence of a user row — decides. A dead session must throw.
        h.authStoreState.user = USER
        h.pbAuthStore.isValid = false
        expect(() => renderHook(() => useAuth())).toThrow(AuthRequiredError)
    })

    it('returns the authenticated user when the session is valid', () => {
        setAuthed()
        const { result } = renderHook(() => useAuth())
        expect(result.current.user.id).toBe('u1')
        expect(result.current.isLoggedIn).toBe(true)
    })
})
