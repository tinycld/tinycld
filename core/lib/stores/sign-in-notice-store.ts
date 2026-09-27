import { create } from '@tinycld/core/lib/store'
import { useEffect, useRef } from 'react'

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
    /** The sign-in screen that showed the current notice; null until one did. */
    shownBy: object | null
    setNotice: (text: string) => void
    /**
     * Called by a mounted sign-in screen. The first screen to show a notice
     * owns it. A different screen claiming it means the owner went away
     * without a sign-in, so the notice has been seen and is cleared.
     */
    claim: (screen: object) => void
    clear: () => void
}

export const useSignInNoticeStore = create<SignInNoticeStoreState>()(set => ({
    notice: null,
    shownBy: null,
    setNotice: text => set({ notice: text, shownBy: null }),
    claim: screen =>
        set(s => {
            if (s.notice === null || s.shownBy === screen) return s
            if (s.shownBy === null) return { shownBy: screen }
            return { notice: null, shownBy: null }
        }),
    clear: () => set({ notice: null, shownBy: null }),
}))

/**
 * The notice for one mounted sign-in screen. It is cleared once shown, not
 * when the screen unmounts: an unmount clear would drop a notice the user has
 * not read when the gate remounts, and StrictMode runs the cleanup at once.
 * The screen's identity is a ref, which a StrictMode re-run keeps and a real
 * remount replaces.
 */
export function useSignInNotice(): string | null {
    const screen = useRef({}).current
    const notice = useSignInNoticeStore(s =>
        s.shownBy === null || s.shownBy === screen ? s.notice : null
    )
    const claim = useSignInNoticeStore(s => s.claim)
    useEffect(() => {
        claim(screen)
    }, [claim, screen])
    return notice
}
