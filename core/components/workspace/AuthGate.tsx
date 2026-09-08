import { setPendingRoute } from '@tinycld/core/lib/pending-route'
import { useUnstableGlobalHref } from 'expo-router'
import { useEffect } from 'react'
import { LoginModal } from './LoginModal'

export function AuthGate() {
    // Second of two capture points. pending-route.ts snapshots the web entry URL
    // at import time, because a deep-linked signed-out load collapses the address
    // bar to the bare app root before this component ever renders. This one
    // covers the cases where the MOUNTED route really is the destination: an
    // in-place session expiry, and the OAuth consent screen (which renders the
    // gate itself and needs its ?user_code= back).
    //
    // The two can't fight: setPendingRoute rejects the app root, so the '/a' this
    // sees on a collapsed deep link is discarded and the entry snapshot stands.
    //
    // useUnstableGlobalHref, not usePathname — mail/drive/boards encode view
    // state in the query string.
    const href = useUnstableGlobalHref()

    useEffect(() => {
        setPendingRoute(href)
    }, [href])

    return <LoginModal />
}
