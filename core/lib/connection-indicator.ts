import type { SyncStatus } from 'pbtsdb'

// Pure state for the app shell's connection notice (useConnectionIndicator).

/**
 * - `offline`: the device has no network.
 * - `reconnecting`: online, but live updates are reconnecting or lists are
 *   retrying their loads.
 * - `unreachable`: online, and the server has not answered for long enough —
 *   or the health check says it is down — that the user should see options.
 */
export type ConnectionIndicatorState = 'offline' | 'reconnecting' | 'unreachable' | 'hidden'

export interface ConnectionSignals {
    isOnline: boolean
    /** False once the health check / sustained request failures say the server is down. */
    isServerReachable: boolean
    isSignedIn: boolean
    /** False on a page that turned realtime off on purpose (an embed). */
    isRealtimeEnabled: boolean
    /** pbtsdb's sync status, narrowed by syncOutageOf. */
    sync: SyncOutage
}

/**
 * The part of pbtsdb's sync status the notice reads: null when live updates
 * and loads are healthy, else when the outage began (epoch ms), or `undated`
 * when a load retries without a start time.
 *
 * A primitive, so a `useSyncExternalStore` over it compares with Object.is
 * and a change the notice does not read (`loads.failed`, a retry attempt)
 * does not render the indicator again.
 */
export type SyncOutage = number | 'undated' | null

/** How long a problem must last before the notice shows it at all. */
export const CONNECTION_INDICATOR_DELAY_MS = 1500

/** How long a reconnect must last before the notice offers options. */
export const CONNECTION_ESCALATE_AFTER_MS = 20_000

export interface ConnectionNotice {
    state: ConnectionIndicatorState
    /**
     * When a `reconnecting` notice becomes `unreachable` if nothing changes,
     * epoch ms; null when no escalation is pending.
     */
    escalatesAt: number | null
}

/**
 * The sync outage in pbtsdb's status, dated from its own timestamps: the
 * realtime stream's `since` and the oldest retrying load's `failingSince`.
 *
 * An outage means the live-update stream dropped and is retrying, or a list's
 * load is sleeping in its retry backoff. A load the server refused (a 403 or
 * 404, `loads.failed`) is an answer, not an outage, and a disabled stream is
 * deliberate (logged out, an embed, nothing subscribed yet), so neither counts.
 */
export function syncOutageOf(sync: SyncStatus): SyncOutage {
    const { realtime, loads } = sync
    const times: number[] = []
    if (realtime.state === 'reconnecting') times.push(realtime.since)
    if (loads.retrying > 0 && loads.failingSince !== undefined) times.push(loads.failingSince)
    if (times.length > 0) return Math.min(...times)
    return loads.retrying > 0 ? 'undated' : null
}

/**
 * What the notice should say at `now`, before the anti-flicker delay.
 *
 * Reconnecting means a sync outage (see syncOutageOf). Without a network the
 * notice says offline and offers nothing: no server option helps until the
 * network is back.
 */
export function connectionNotice(signals: ConnectionSignals, now: number): ConnectionNotice {
    const hidden: ConnectionNotice = { state: 'hidden', escalatesAt: null }
    if (!signals.isSignedIn || !signals.isRealtimeEnabled) return hidden
    if (!signals.isOnline) return { state: 'offline', escalatesAt: null }
    if (!signals.isServerReachable) return { state: 'unreachable', escalatesAt: null }

    const since = signals.sync
    if (since === null) return hidden
    if (since === 'undated') return { state: 'reconnecting', escalatesAt: null }
    const escalatesAt = since + CONNECTION_ESCALATE_AFTER_MS
    return now >= escalatesAt
        ? { state: 'unreachable', escalatesAt: null }
        : { state: 'reconnecting', escalatesAt }
}

/** The pill's text: the server's host where the app chose a server, plain words on web. */
export function noticeLabel(state: ConnectionIndicatorState, serverHost: string | null): string {
    if (state === 'offline') return 'Offline — waiting for a connection'
    if (state === 'reconnecting') return 'Reconnecting…'
    if (state === 'unreachable') {
        return serverHost
            ? `Can't reach ${serverHost}. Tap for options`
            : "Can't reach the server. Tap for options"
    }
    return ''
}

/** The host of a server address, for display; null when it does not parse. */
export function serverHostLabel(address: string | null): string | null {
    if (!address) return null
    try {
        return new URL(address).host || null
    } catch {
        return null
    }
}
