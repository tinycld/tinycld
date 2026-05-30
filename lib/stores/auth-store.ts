import AsyncStorage from '@react-native-async-storage/async-storage'
import { captureException } from '@tinycld/core/lib/errors'
import {
    authStoreReady,
    fetchAndSeedUserOrg,
    getUserFromAuthStore,
    PB_SERVER_ADDR,
    pb,
    preloadStores,
    seedUserOrg,
} from '@tinycld/core/lib/pocketbase'
import { create } from '@tinycld/core/lib/store'
import { useWorkspaceStore } from '@tinycld/core/lib/stores/workspace-store'
import type { UserSession } from '@tinycld/core/lib/types'
import type { Orgs, UserOrg, Users } from '@tinycld/core/types/pbSchema'

interface UserOrgExpanded extends UserOrg {
    expand?: { org?: Orgs }
}

type AuthenticatedUser = UserSession

type LoginResult = {
    user: AuthenticatedUser | null
    error: string | null
}

const PRIMARY_ORG_STORAGE_KEY = 'tinycld_primary_org'

async function savePrimaryOrgToStorage(orgSlug: string): Promise<void> {
    try {
        await AsyncStorage.setItem(PRIMARY_ORG_STORAGE_KEY, orgSlug)
    } catch {
        // Storage might not be available
    }
}

export async function loadPrimaryOrgFromStorage(): Promise<string | null> {
    try {
        return await AsyncStorage.getItem(PRIMARY_ORG_STORAGE_KEY)
    } catch {
        return null
    }
}

async function clearPrimaryOrgStorage(): Promise<void> {
    try {
        await AsyncStorage.removeItem(PRIMARY_ORG_STORAGE_KEY)
    } catch {
        // Storage might not be available
    }
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

                const primaryOrgSlug = await loadPrimaryOrgFromStorage()
                const currentUser = getUserFromAuthStore(primaryOrgSlug)

                if (currentUser) {
                    await fetchAndSeedUserOrg()
                    await preloadStores()
                    set({ user: currentUser })
                }
            } catch (err) {
                captureException('auth-store.initAuth hydration failed', err)
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
            const authData = await pb.collection('users').authWithPassword<
                Users & {
                    expand?: {
                        user_org_via_user?: UserOrgExpanded[]
                    }
                }
            >(identifier, password, {
                expand: 'user_org_via_user.org',
            })
            const userOrgs = authData.record.expand?.user_org_via_user ?? []
            const firstUserOrgWithSlug = userOrgs.find(uo => uo.expand?.org?.slug)

            if (!firstUserOrgWithSlug?.expand?.org) {
                pb.authStore.clear()
                return {
                    user: null,
                    error: 'No organization associated with this account',
                }
            }

            const primaryOrgSlug = firstUserOrgWithSlug.expand.org.slug

            const metadata = (authData.record as Users & { metadata?: Record<string, unknown> })
                .metadata
            const authenticatedUser: AuthenticatedUser = {
                id: authData.record.id,
                name: authData.record.name,
                email: authData.record.email,
                primaryOrgSlug,
                isDemo: !!(authData.record as Users & { is_demo?: boolean }).is_demo,
                isBetaTester: !!metadata?.isBetaTester,
            }

            const { expand: _, ...userOrgRecord } = firstUserOrgWithSlug
            await seedUserOrg(authData.record, firstUserOrgWithSlug.expand.org, userOrgRecord)

            await savePrimaryOrgToStorage(primaryOrgSlug)
            set({ user: authenticatedUser })
            await preloadStores()

            return { user: authenticatedUser, error: null }
        } catch (error) {
            const message = error instanceof Error ? error.message : 'Failed to sign in'
            return { user: null, error: message }
        }
    },

    logout: () => {
        pb.realtime.unsubscribe()
        pb.authStore.clear()
        clearPrimaryOrgStorage()
        // Wipe per-device rail deep-links so a second user signing in on
        // the same device doesn't inherit the previous user's last-opened
        // file references.
        useWorkspaceStore.setState({ lastPackageHref: {} })
        set({ user: null })
    },

    refreshUser: async () => {
        const primaryOrgSlug = await loadPrimaryOrgFromStorage()
        const currentUser = getUserFromAuthStore(primaryOrgSlug)
        if (!currentUser) {
            set({ user: null })
            await clearPrimaryOrgStorage()
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

            // Expand user_org_via_user.org to get the primary org slug for the guest.
            const expanded = await pb.collection('users').getOne<
                Users & {
                    expand?: {
                        user_org_via_user?: UserOrgExpanded[]
                    }
                }
            >(data.record.id, { expand: 'user_org_via_user.org' })

            const userOrgs = expanded.expand?.user_org_via_user ?? []
            const firstUserOrgWithSlug = userOrgs.find(uo => uo.expand?.org?.slug)

            if (!firstUserOrgWithSlug?.expand?.org) {
                return { user: null, error: 'No organization associated with this account' }
            }

            const primaryOrgSlug = firstUserOrgWithSlug.expand.org.slug

            const authenticatedUser: AuthenticatedUser = {
                id: data.record.id,
                name: data.record.name,
                email: data.record.email,
                primaryOrgSlug,
                isDemo: false,
                isBetaTester: false,
            }

            const { expand: _, ...userOrgRecord } = firstUserOrgWithSlug
            await seedUserOrg(data.record, firstUserOrgWithSlug.expand.org, userOrgRecord)

            await savePrimaryOrgToStorage(primaryOrgSlug)
            set({ user: authenticatedUser })
            await preloadStores()

            return { user: authenticatedUser, error: null }
        } catch (error) {
            const message = error instanceof Error ? error.message : 'Failed to verify code'
            return { user: null, error: message }
        }
    },
}))
