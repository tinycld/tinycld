import type { ConnectivityState } from '@tinycld/core/lib/stores/connectivity-store'

// Pure pieces of useReloadOnResume, split out so they test without React or
// React Native.

/**
 * Out of touch at least this long, the app reloads every live query when it
 * returns. A shorter gap is left to the realtime connection: the server
 * replays the events it missed, and refuses (so pbtsdb reloads) only when it
 * cannot.
 */
export const RELOAD_AFTER_AWAY_MS = 15 * 60 * 1000

type Reachability = Pick<ConnectivityState, 'isOnline' | 'isServerReachable'>

/** The app went to the background. */
export function isBackground(previous: string, next: string): boolean {
    return previous === 'active' && next !== 'active'
}

/** The network, or our server, went away. */
export function isDisconnect(previous: Reachability, next: Reachability): boolean {
    return (
        (previous.isOnline && !next.isOnline) ||
        (previous.isServerReachable && !next.isServerReachable)
    )
}

/** In the foreground, online, and able to reach our server. */
export function isInTouch(appState: string, connectivity: Reachability): boolean {
    return appState === 'active' && connectivity.isOnline && connectivity.isServerReachable
}

/**
 * Measures how long the app was out of touch: from the first time it left
 * (to the background, or off the network) until it is back.
 */
export function createAwayClock(now: () => number = Date.now) {
    let since: number | null = null
    return {
        leave() {
            since ??= now()
        },
        /** How long the app was away; the next absence is measured from zero. */
        back(): number {
            const away = since === null ? 0 : now() - since
            since = null
            return away
        },
    }
}

type AwayClock = ReturnType<typeof createAwayClock>

/**
 * What to do once the app may be back: nothing while it is still out of
 * touch (a later return then measures the whole absence), a full reload
 * after a long absence when `reloadAfterLongAbsence` is set, and otherwise
 * only a realtime reconnect.
 */
export function createCatchUp(deps: {
    inTouch: () => boolean
    away: AwayClock
    reloadAfterLongAbsence: boolean
    reload: () => Promise<void>
    reconnect: () => void
}) {
    return async () => {
        if (!deps.inTouch()) return
        const longAbsence = deps.away.back() >= RELOAD_AFTER_AWAY_MS
        if (longAbsence && deps.reloadAfterLongAbsence) await deps.reload()
        else deps.reconnect()
    }
}

/** The app came back to the foreground. */
export function isResume(previous: string, next: string): boolean {
    return previous !== 'active' && next === 'active'
}

/** The network, or our server, came back. */
export function isReconnect(previous: Reachability, next: Reachability): boolean {
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
