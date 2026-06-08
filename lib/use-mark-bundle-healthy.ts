import { markBundleHealthy } from '@tinycld/core/lib/mark-bundle-healthy'
import { useEffect } from 'react'

// useMarkBundleHealthy marks the active OTA bundle healthy on the caller's FIRST
// commit — the earliest point that proves the bundle loaded and rendered without
// crashing, which is exactly the signal native crash-rollback waits for. (Marking
// any earlier, e.g. at module-eval, would defeat rollback: a bundle that crashes
// during render would already be flagged healthy.) The `[]` effect runs on mount
// regardless of which gate branch the layout renders, so the placeholder render
// counts too. The native side additionally tolerates a couple of un-marked boots
// before rolling back (see Store.rollbackAfterLaunches), covering a render that's
// force-quit before this fires.
export function useMarkBundleHealthy(): void {
    useEffect(() => {
        markBundleHealthy()
    }, [])
}
