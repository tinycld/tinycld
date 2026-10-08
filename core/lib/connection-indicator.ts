import type { SyncStatus } from 'pbtsdb'

// Pure state for the app shell's connection indicator (useConnectionIndicator).

export type ConnectionIndicatorState = 'offline' | 'reconnecting' | 'hidden'

export interface ConnectionSignals {
    isOnline: boolean
    isSignedIn: boolean
    /** False on a page that turned realtime off on purpose (an embed). */
    isRealtimeEnabled: boolean
    sync: SyncStatus
}

/**
 * What the indicator should say right now, before the anti-flicker delay.
 *
 * Reconnecting means pbtsdb's live-update stream dropped and is retrying, or a
 * list's load is sleeping in its retry backoff. A load that ended in a 403 or
 * 404 (`loads.failed`) is an answer, not an outage, and a disabled stream is
 * deliberate (logged out, an embed, nothing subscribed yet), so neither shows.
 */
export function connectionIndicatorState(signals: ConnectionSignals): ConnectionIndicatorState {
    if (!signals.isSignedIn || !signals.isRealtimeEnabled) return 'hidden'
    if (!signals.isOnline) return 'offline'
    const { realtime, loads } = signals.sync
    if (realtime.state === 'reconnecting' || loads.retrying > 0) return 'reconnecting'
    return 'hidden'
}

/** How long a problem must last before the indicator shows it. */
export const CONNECTION_INDICATOR_DELAY_MS = 1500
