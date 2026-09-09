// @vitest-environment happy-dom
import { beforeEach, describe, expect, it, vi } from 'vitest'

// useExpoPushRegistration is mounted in the org layout, which renders BEFORE
// the auth gate — so it runs for signed-out visitors too. It originally called
// useAuth() with no options, and throwIfAnon defaults to TRUE: that threw
// AuthRequiredError during render, crashing the layout into the error boundary.
// A signed-out deep link then showed "Something went wrong" instead of the
// login form, which is exactly how the login-return-to e2e spec failed.
//
// The effect's own `user?.id` guard cannot save this — a render-time throw
// happens before any effect runs.

// A faithful stand-in for the real useAuth: throwIfAnon DEFAULTS TO TRUE and
// throws when there is no user (core/lib/auth.tsx). Mocking it as a plain
// vi.fn() that always returns cleanly would let the bug through — the hook
// could go back to a bare useAuth() and the test would still pass.
const AuthRequiredError = vi.hoisted(() => class AuthRequiredError extends Error {})
const anonUser = vi.hoisted(() => ({ current: true }))
const useAuth = vi.hoisted(() =>
    vi.fn((options?: { throwIfAnon: boolean }) => {
        const user = anonUser.current ? null : { id: 'user-1' }
        if ((options?.throwIfAnon ?? true) && !user) throw new AuthRequiredError()
        return { user, isLoggedIn: !!user }
    })
)
const registerExpoPushToken = vi.hoisted(() => vi.fn())

// Registration is native-only (it early-returns on web), so the suite runs as
// iOS — otherwise every assertion below would pass vacuously against the web
// no-op. Matches the bundle-sentinel spec's approach.
vi.mock('react-native', () => ({ Platform: { OS: 'ios' } }))
vi.mock('@tinycld/core/lib/auth', () => ({ useAuth, AuthRequiredError }))
vi.mock('@tinycld/core/lib/expo-push', () => ({ registerExpoPushToken }))

import { render } from '@testing-library/react'
import {
    resetExpoPushRegistration,
    useExpoPushRegistration,
} from '@tinycld/core/lib/use-expo-push-registration'

function Probe() {
    useExpoPushRegistration()
    return null
}

describe('useExpoPushRegistration', () => {
    beforeEach(() => {
        vi.clearAllMocks()
        resetExpoPushRegistration()
    })

    it('renders for a signed-out visitor instead of throwing', () => {
        anonUser.current = true
        expect(() => render(<Probe />)).not.toThrow()
        expect(registerExpoPushToken).not.toHaveBeenCalled()
    })

    it('asks useAuth not to throw on anonymous', () => {
        anonUser.current = true
        render(<Probe />)
        expect(useAuth).toHaveBeenCalledWith({ throwIfAnon: false })
    })

    it('still registers the token once a user is signed in', () => {
        anonUser.current = false
        render(<Probe />)
        expect(registerExpoPushToken).toHaveBeenCalledWith('user-1')
    })
})
