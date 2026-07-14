import { pb, refreshAuth } from '@tinycld/core/lib/pocketbase'
import { useAuthStore } from '@tinycld/core/lib/stores/auth-store'
import type { ReactNode } from 'react'
import { useEffect } from 'react'
import { AppState, type AppStateStatus, Platform } from 'react-native'

export { loadPrimaryOrgFromStorage } from '@tinycld/core/lib/stores/auth-store'

export class AuthRequiredError extends Error {
    constructor(message = 'Authentication required') {
        super(message)
        this.name = 'AuthRequiredError'
    }
}

interface UserShape {
    id: string
    name: string
    email: string
    primaryOrgSlug?: string
    isDemo: boolean
}

type AuthActions = {
    login: (
        email: string,
        password: string
    ) => Promise<{
        user: UserShape | null
        error: string | null
    }>
    logout: () => void
}

type AuthenticatedContext = AuthActions & {
    isLoggedIn: true
    user: UserShape
}

type AuthContextType =
    | (AuthActions & {
          isLoggedIn: true
          user: UserShape
          isInitializing: boolean
      })
    | (AuthActions & {
          isLoggedIn: false
          user: null
          isInitializing: boolean
      })

// Renew the session well before the token's `exp`, so an active user never
// crosses expiry mid-session. PB's default auth-token duration is 7 days; an
// hourly refresh keeps a token in continuous use fresh with a wide safety
// margin, and refreshing on foreground/tab-focus covers a session that sat idle
// (interval starved while backgrounded) and comes back after a long pause.
const REFRESH_INTERVAL_MS = 60 * 60 * 1000

export function AuthProvider({ children }: { children: ReactNode }) {
    const initAuth = useAuthStore(s => s.initAuth)

    useEffect(() => {
        const unsubscribe = initAuth()
        return unsubscribe
    }, [initAuth])

    useEffect(() => {
        const interval = setInterval(() => {
            void refreshAuth()
        }, REFRESH_INTERVAL_MS)

        if (Platform.OS === 'web') {
            const onVisible = () => {
                if (typeof document !== 'undefined' && document.visibilityState === 'visible') {
                    void refreshAuth()
                }
            }
            if (typeof document !== 'undefined') {
                document.addEventListener('visibilitychange', onVisible)
            }
            return () => {
                clearInterval(interval)
                if (typeof document !== 'undefined') {
                    document.removeEventListener('visibilitychange', onVisible)
                }
            }
        }

        const onAppState = (next: AppStateStatus) => {
            if (next === 'active') void refreshAuth()
        }
        const subscription = AppState.addEventListener('change', onAppState)
        return () => {
            clearInterval(interval)
            subscription.remove()
        }
    }, [])

    return <>{children}</>
}

export function useAuth(): AuthenticatedContext
export function useAuth(options: { throwIfAnon: true }): AuthenticatedContext
export function useAuth(options: { throwIfAnon: false }): AuthContextType
export function useAuth(options?: {
    throwIfAnon: boolean
}): AuthenticatedContext | AuthContextType {
    const user = useAuthStore(s => s.user)
    const hasHydrated = useAuthStore(s => s.hasHydrated)
    const login = useAuthStore(s => s.login)
    const logout = useAuthStore(s => s.logout)

    // Gate on token VALIDITY, not mere presence: an expired token still sits in
    // the store (expiry doesn't clear it), so `!!pb.authStore.token` would keep
    // reporting "logged in" for a dead session whose every request fails. Using
    // isValid (checks the JWT `exp`) means an expired token routes to the login
    // gate instead of a broken half-session. refreshAuth clears a truly-dead
    // token on the next boot/interval/focus tick, but this makes the gate
    // correct even in the window before that fires.
    const isLoggedIn = !!user && pb.authStore.isValid
    const throwIfAnon = options?.throwIfAnon ?? true

    const context: AuthContextType = isLoggedIn
        ? { login, logout, user, isLoggedIn: true, isInitializing: !hasHydrated }
        : { login, logout, user: null, isLoggedIn: false, isInitializing: !hasHydrated }

    if (throwIfAnon) {
        // The throwIfAnon contract is "the caller requires an authenticated
        // user". When there is none we throw AuthRequiredError so the nearest
        // error boundary handles it, rather than fabricating a blank-id user
        // that downstream hooks would run queries against (user.id === '').
        // Anon-allowed callers pass throwIfAnon: false and are unaffected.
        if (!context.user) {
            throw new AuthRequiredError()
        }
        return context as AuthenticatedContext
    }

    return context
}
