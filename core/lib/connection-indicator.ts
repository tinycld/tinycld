// Pure state for the app shell's connection indicator (useConnectionIndicator).

export type ConnectionIndicatorState = 'offline' | 'reconnecting' | 'hidden'

export interface ConnectionSignals {
    isOnline: boolean
    isServerReachable: boolean
    isRequestFailing: boolean
    isSignedIn: boolean
    /** False on a page that turned realtime off on purpose (an embed). */
    isRealtimeEnabled: boolean
}

/** What the indicator should say right now, before the anti-flicker delay. */
export function connectionIndicatorState(signals: ConnectionSignals): ConnectionIndicatorState {
    if (!signals.isSignedIn || !signals.isRealtimeEnabled) return 'hidden'
    if (!signals.isOnline) return 'offline'
    if (!signals.isServerReachable || signals.isRequestFailing) return 'reconnecting'
    return 'hidden'
}

/** How long a problem must last before the indicator shows it. */
export const CONNECTION_INDICATOR_DELAY_MS = 1500
