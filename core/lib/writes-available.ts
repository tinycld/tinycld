// Pure part of useWritesAvailable.

export interface WritesAvailability {
    available: boolean
    /** Why a write cannot be saved right now; empty when it can. */
    reason: string
}

/**
 * Whether a change can reach the server now. Live updates reconnecting do not
 * matter here — a save is a plain request — but no network, or a server the
 * health check and repeated failures say is down, do.
 */
export function writesAvailability(signals: {
    isOnline: boolean
    isServerReachable: boolean
}): WritesAvailability {
    if (!signals.isOnline) {
        return { available: false, reason: "You're offline — changes can't be saved right now" }
    }
    if (!signals.isServerReachable) {
        return {
            available: false,
            reason: "Can't reach the server — changes can't be saved right now",
        }
    }
    return { available: true, reason: '' }
}
