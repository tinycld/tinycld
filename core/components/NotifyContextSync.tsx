import { useAuth } from '@tinycld/core/lib/auth'
import { clearNotifyContext, setNotifyContext } from '@tinycld/core/lib/notify/context'
import { useEffect } from 'react'

/**
 * Syncs the current user id into the module-level notify context so non-hook
 * callers (e.g. notify.emit) can reach it. Mounted once inside the app layout.
 *
 * This used to also require an orgId, which is what made every bell
 * notification dead app-wide: that field was permanently '', so the guard
 * always bailed, the context was never set, and each dispatch no-opped while
 * firing captureException('notify.bell.no_context') — a Sentry report on a
 * code path that could never work. The field is gone entirely now; the user is
 * the only identifier a notification needs. notify-context-sync.test.tsx is
 * the regression guard.
 */
export function NotifyContextSync() {
    const auth = useAuth({ throwIfAnon: false })
    const userId = auth.isLoggedIn ? auth.user.id : null

    useEffect(() => {
        if (!userId) return
        setNotifyContext({ userId })
        return () => clearNotifyContext()
    }, [userId])

    return null
}
