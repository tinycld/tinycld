import type { ConnectivityState } from '@tinycld/core/lib/stores/connectivity-store'

// Pure pieces of useReloadOnResume, split out so they test without React or
// React Native.

/** The app came back to the foreground. */
export function isResume(previous: string, next: string): boolean {
    return previous !== 'active' && next === 'active'
}

/** The network, or our server, came back. */
export function isReconnect(
    previous: Pick<ConnectivityState, 'isOnline' | 'isServerReachable'>,
    next: Pick<ConnectivityState, 'isOnline' | 'isServerReachable'>
): boolean {
    return (
        (next.isOnline && !previous.isOnline) ||
        (next.isServerReachable && !previous.isServerReachable)
    )
}

/**
 * Coalesces triggers that arrive together — a phone waking up reports both
 * "active" and "online" — into one run, `delayMs` after the last of them.
 */
export function createDebouncedTrigger(
    run: () => Promise<void>,
    delayMs: number,
    onError: (error: unknown) => void
) {
    let timer: ReturnType<typeof setTimeout> | null = null
    return {
        trigger() {
            if (timer) clearTimeout(timer)
            timer = setTimeout(() => {
                timer = null
                run().catch(onError)
            }, delayMs)
        },
        cancel() {
            if (timer) clearTimeout(timer)
            timer = null
        },
    }
}
