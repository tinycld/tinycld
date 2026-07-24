import { captureException } from '@tinycld/core/lib/errors'
import { unregisterExpoPushToken } from '@tinycld/core/lib/expo-push'
import {
    authStoreReady,
    getUserFromAuthStore,
    PB_SERVER_ADDR,
    pb,
    preloadStores,
    refreshAuth,
    resetSessionState,
    seedUser,
} from '@tinycld/core/lib/pocketbase'
import { create } from '@tinycld/core/lib/store'
import { useWorkspaceStore } from '@tinycld/core/lib/stores/workspace-store'
import type { UserSession } from '@tinycld/core/lib/types'
import { resetExpoPushRegistration } from '@tinycld/core/lib/use-expo-push-registration'
import { isPushSupported, unsubscribeFromPush } from '@tinycld/core/lib/web-push'
import type { Users } from '@tinycld/core/types/pbSchema'
import { Platform } from 'react-native'

type AuthenticatedUser = UserSession

type LoginResult = {
    user: AuthenticatedUser | null
    error: string | null
}

// Tear down the device's push subscription on logout: remove the browser/device
// push registration AND delete the matching server push_subscriptions row, so a
// signed-out device stops receiving pushes. Resetting resetExpoPushRegistration()
// clears the module-lifetime guard so a second user signing in on the same
// running session re-registers their own token (otherwise the guard would
// suppress it). Best-effort: platform push helpers already swallow their own
// errors, and any failure here must never block logout.
async function teardownPushOnLogout(userId: string, authToken: string): Promise<void> {
    if (Platform.OS === 'web') {
        if (isPushSupported()) await unsubscribeFromPush(userId, authToken)
    } else {
        await unregisterExpoPushToken(userId, authToken)
    }
    resetExpoPushRegistration()
}

type RequestOtpResult = {
    otpId: string | null
    error: string | null
}

type VerifyOtpResult = {
    user: AuthenticatedUser | null
    error: string | null
}

interface AuthStoreState {
    user: AuthenticatedUser | null
    hasHydrated: boolean

    initAuth: () => () => void
    login: (identifier: string, password: string) => Promise<LoginResult>
    logout: () => void
    refreshUser: () => Promise<void>
    requestShareOtp: (token: string, email: string) => Promise<RequestOtpResult>
    verifyShareOtp: (
        token: string,
        email: string,
        code: string,
        otpId: string
    ) => Promise<VerifyOtpResult>
    startDemo: (serverAddr: string) => Promise<LoginResult>
}

export const useAuthStore = create<AuthStoreState>()((set, get) => ({
    user: null,
    hasHydrated: false,

    initAuth: () => {
        // Hydrate auth and preload server data BEFORE flipping isLoggedIn.
        // The previous order set user first and then awaited preloads, which
        // left a window where components mounted with isLoggedIn=true against
        // empty TanStack DB collections — rendering as "everything empty"
        // until the user signed out and back in. We also wrap everything so
        // a failed preload (timeout, transient 5xx, EventSource hiccup) can't
        // strand the app with hasHydrated=false forever.
        const hydrate = async () => {
            try {
                await authStoreReady

                // Renew (or, if the stored token is already dead, clear) the
                // session BEFORE deriving the user. A token that expired while
                // the app was closed would otherwise hydrate a "logged in" user
                // whose every request fails — the stale-session bug. refreshAuth
                // clears the store on an authoritative rejection, so
                // getUserFromAuthStore then returns null and the app gates to
                // login instead of a broken half-session.
                await refreshAuth()

                const currentUser = getUserFromAuthStore()

                if (currentUser) {
                    await preloadStores()
                    set({ user: currentUser })
                }
            } catch (err) {
                captureException('auth-store.initAuth.hydrate', err)
            } finally {
                set({ hasHydrated: true })
            }
        }

        hydrate()

        const unsubscribe = pb.authStore.onChange(() => {
            get().refreshUser()
        })
        return unsubscribe
    },

    login: async (identifier, password) => {
        pb.authStore.clear()
        try {
            const authData = await pb
                .collection('users')
                .authWithPassword<Users>(identifier, password)

            const metadata = (authData.record as Users & { metadata?: Record<string, unknown> })
                .metadata
            const authenticatedUser: AuthenticatedUser = {
                id: authData.record.id,
                name: authData.record.name,
                email: authData.record.email,
                isDemo: !!(authData.record as Users & { is_demo?: boolean }).is_demo,
                isBetaTester: !!metadata?.isBetaTester,
            }

            await seedUser(authData.record)
            set({ user: authenticatedUser })
            await preloadStores()

            return { user: authenticatedUser, error: null }
        } catch (error) {
            const message = error instanceof Error ? error.message : 'Failed to sign in'
            return { user: null, error: message }
        }
    },

    logout: () => {
        // Capture the userId + token while pb.authStore is still authed — push
        // teardown deletes this user's server subscription row and needs both.
        // The token is forwarded as an Authorization header inside teardown so
        // the delete stays authorized after pb.authStore.clear() below (which
        // runs synchronously, before teardown's async PB requests dispatch).
        const userId = get().user?.id
        const authToken = pb.authStore.token
        pb.realtime.unsubscribe()
        // Fire-and-forget push teardown: unsubscribe the device and delete the
        // server push_subscriptions row, then reset the module-lifetime
        // registration guard so a second user on this same session re-registers.
        if (userId) {
            teardownPushOnLogout(userId, authToken).catch(err =>
                captureException('auth-store.logout.teardownPush', err)
            )
        }
        pb.authStore.clear()
        // Wipe per-device rail deep-links so a second user signing in on
        // the same device doesn't inherit the previous user's last-opened
        // file references.
        useWorkspaceStore.setState({ lastPackageHref: {} })
        // Drop per-session in-memory caches (pbtsdb stores + React Query,
        // incl. the pb file token) so the next user signing in on this SPA
        // instance can't read the previous user's rows or reuse their token.
        resetSessionState().catch(err =>
            captureException('auth-store.logout.resetSessionState', err)
        )
        set({ user: null })
    },

    refreshUser: async () => {
        const currentUser = getUserFromAuthStore()
        if (!currentUser) {
            set({ user: null })
            return
        }
        set({ user: currentUser })
    },

    requestShareOtp: async (token, email) => {
        try {
            const res = await fetch(`${PB_SERVER_ADDR}/api/drive/share-link/${token}/otp-request`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ email }),
            })
            if (res.ok) {
                const data = (await res.json()) as { ok: true; otp_id: string }
                return { otpId: data.otp_id, error: null }
            }
            let errorMsg: string
            try {
                const body = (await res.json()) as { error?: string }
                errorMsg = body.error ?? res.statusText
            } catch {
                errorMsg = res.statusText
            }
            return { otpId: null, error: errorMsg }
        } catch {
            return { otpId: null, error: 'network error' }
        }
    },

    verifyShareOtp: async (token, email, code, otpId) => {
        try {
            const res = await fetch(`${PB_SERVER_ADDR}/api/drive/share-link/${token}/otp-verify`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ email, code, otp_id: otpId }),
            })
            if (!res.ok) {
                let errorMsg: string
                try {
                    const body = (await res.json()) as { error?: string }
                    errorMsg = body.error ?? res.statusText
                } catch {
                    errorMsg = res.statusText
                }
                return { user: null, error: errorMsg }
            }

            const data = (await res.json()) as { token: string; record: Users }
            // Save into pb.authStore WITHOUT clearing first — clearing would log out
            // an already-signed-in member who happens to be opening a share link.
            pb.authStore.save(data.token, data.record as never)

            const authenticatedUser: AuthenticatedUser = {
                id: data.record.id,
                name: data.record.name,
                email: data.record.email,
                isDemo: false,
                isBetaTester: false,
            }

            await seedUser(data.record)
            set({ user: authenticatedUser })
            await preloadStores()

            return { user: authenticatedUser, error: null }
        } catch (error) {
            const message = error instanceof Error ? error.message : 'Failed to verify code'
            return { user: null, error: message }
        }
    },

    startDemo: async serverAddr => {
        // The marketing site's "Start a demo" tap routes here on devices with the
        // app installed (universal/app link). We hit the same unauthenticated
        // endpoint the web CTA uses and adopt the returned PocketBase envelope.
        pb.authStore.clear()
        try {
            const res = await fetch(`${serverAddr}/api/demo/start`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
            })
            if (!res.ok) {
                return { user: null, error: `Server returned HTTP ${res.status}` }
            }

            const data = (await res.json()) as { token?: string; record?: Users }
            if (!data.token || !data.record?.id) {
                return { user: null, error: 'Malformed demo response' }
            }

            pb.authStore.save(data.token, data.record as never)

            const authenticatedUser: AuthenticatedUser = {
                id: data.record.id,
                name: data.record.name,
                email: data.record.email,
                isDemo: true,
                isBetaTester: false,
            }

            await seedUser(data.record)
            set({ user: authenticatedUser })
            await preloadStores()

            return { user: authenticatedUser, error: null }
        } catch (error) {
            captureException('demo.start', error)
            const message = error instanceof Error ? error.message : 'Failed to start demo'
            return { user: null, error: message }
        }
    },
}))
