import { create } from '@tinycld/core/lib/store'

/**
 * A message the login form shows above its fields, set by a package before it
 * routes an unauthenticated user to sign-in (e.g. an expired or already-used
 * invite link). Not persisted — the notice belongs to the redirect that just
 * happened, not to a future app launch.
 *
 * Deliberately a store, not a `?message=` URL parameter: a URL parameter lets
 * any link put arbitrary text on the login form, which helps phishing. A
 * store can be set only by code that runs inside the app.
 */
interface SignInNoticeStoreState {
    notice: string | null
    setNotice: (text: string) => void
    clear: () => void
}

export const useSignInNoticeStore = create<SignInNoticeStoreState>()(set => ({
    notice: null,
    setNotice: text => set({ notice: text }),
    clear: () => set({ notice: null }),
}))
