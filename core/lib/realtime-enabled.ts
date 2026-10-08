import { disconnectRealtime, resetRealtime } from 'pbtsdb'
import { pb } from './pocketbase'

// pbtsdb runs its own realtime connection, separate from `pb.realtime`; every
// collection subscribes through it. This module is the one place the app opens
// and closes that connection.
//
// Realtime is normally on and needs no switch: pbtsdb subscribes a collection
// the first time something reads it, and the share token rides along via
// `subscribeOptions` (see pocketbase.ts), so even an anonymous share-link
// visitor gets live updates for free.
//
// An EMBED is the case where "for free" is the wrong default. A board framed on
// someone else's page would hold an open socket per viewer, on traffic its
// owner does not control and cannot see, so an embed subscribes only when the
// link says to (`embed_live`). The switch is document-wide: an embed renders
// nothing but a read-only board, and keeping its data path identical to a
// member's is what the share-link design exists to preserve.
//
// `disconnectRealtime(pb)` closes the connection and keeps it closed until
// `resetRealtime(pb)`; collections keep working over REST and keep their topics
// registered, so reopening resumes every topic and reloads every ready
// collection.
//
// A module-level variable, not a store, for the same reason share-token.ts
// gives: the readers are not hooks.

let realtimeEnabled = true

/**
 * Turn realtime on or off for this document. Safe to call before anything has
 * subscribed: a disconnected client stays closed until realtime is turned on.
 */
export function setRealtimeEnabled(enabled: boolean) {
    if (realtimeEnabled === enabled) return
    realtimeEnabled = enabled
    if (enabled) resetRealtime(pb)
    else disconnectRealtime(pb)
}

/** Sign-out, or a server switch: close the connection and keep it closed. */
export function stopRealtime() {
    disconnectRealtime(pb)
}

/**
 * Sign-in, or a refused server switch: reopen the connection under `pb`'s
 * current auth and address — unless this document has realtime turned off.
 */
export function restartRealtime() {
    if (realtimeEnabled) resetRealtime(pb)
}

/** False on a page that turned realtime off on purpose, such as an embed. */
export function isRealtimeEnabled(): boolean {
    return realtimeEnabled
}
