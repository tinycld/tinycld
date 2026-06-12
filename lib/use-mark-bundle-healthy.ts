import { markBundleHealthy } from '@tinycld/core/lib/mark-bundle-healthy'
import { useEffect } from 'react'

// useMarkBundleHealthy marks the active OTA bundle healthy — the signal native
// crash-rollback waits for — on the caller's first commit. It must be mounted
// only AFTER the real provider tree (Providers) has committed: that's the
// earliest point that proves the bundle didn't just render a placeholder but
// actually initialized auth, the data layer, and the stores without crashing.
// Marking earlier (e.g. from the top-level layout, which also renders the blank
// gate placeholder) would flag a bundle healthy before any real app code ran, so
// a bundle that mounts the placeholder fine but then crashes in Providers/a real
// screen would never roll back — defeating the safety net. The native side still
// tolerates a couple of un-marked boots before rolling back (see
// Store.rollbackAfterLaunches = 3), which covers a slow first load that's
// force-quit before this fires, so we don't mis-rollback a healthy bundle.
export function useMarkBundleHealthy(): void {
    useEffect(() => {
        markBundleHealthy()
    }, [])
}

// MarkBundleHealthy is the behavior-only marker the layout mounts INSIDE
// <Providers> (only on the 'resolved' gate branch), so the healthy signal fires
// exactly when the real provider tree commits — see useMarkBundleHealthy above
// for why that timing matters. Renders nothing.
export function MarkBundleHealthy(): null {
    useMarkBundleHealthy()
    return null
}
