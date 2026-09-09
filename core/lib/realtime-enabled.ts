import { pb } from './pocketbase'

// Whether this DOCUMENT may hold realtime subscriptions.
//
// Realtime is normally on and needs no switch: pbtsdb subscribes a collection
// the first time something reads it, and the share token rides along via
// `subscribeOptions` (see pocketbase.ts), so even an anonymous share-link
// visitor gets live updates for free.
//
// An EMBED is the case where "for free" is the wrong default. A board framed on
// someone else's page would hold an open socket per viewer, on traffic its
// owner does not control and cannot see, so an embed subscribes only when the
// link says to (`embed_live`).
//
// WHY A DOCUMENT-WIDE SWITCH RATHER THAN A PER-COLLECTION ONE. pbtsdb has no
// "don't subscribe" option — `syncMode` is only eager|on-demand and the
// subscription is intrinsic to a collection — so the alternatives were to fork
// the board's data path or to turn realtime off for the whole page. An embed
// document renders nothing but a read-only board, so the page-wide switch is
// both safe and the one that leaves the board's rendering identical to a
// member's. That identity is the property the whole share-link design exists to
// preserve; a second data path would spend it.
//
// A module-level variable, not a store, for the same reason share-token.ts
// gives: the reader is not a hook. It runs inside the PocketBase client, at
// subscribe time.

let realtimeEnabled = true

/**
 * Turn realtime subscriptions on or off for this document.
 *
 * Disabling also tears down anything already subscribed, because a collection
 * read during the first render may have subscribed before this was called.
 */
export function setRealtimeEnabled(enabled: boolean) {
    if (realtimeEnabled === enabled) return
    realtimeEnabled = enabled
    if (!enabled) {
        // Fire-and-forget: this closes the socket, and a failure to close one
        // that may not even be open is not worth failing a render over. The
        // guard below is what actually keeps it closed.
        void pb.realtime.unsubscribe().catch(() => {})
    }
}

export function isRealtimeEnabled(): boolean {
    return realtimeEnabled
}

/**
 * Refuse new subscriptions while realtime is disabled.
 *
 * Installed over `pb.realtime.subscribe`, which every per-collection
 * `subscribe()` funnels through, so one wrap covers every collection — present
 * and future — without pbtsdb or any caller knowing about it.
 *
 * REJECTS rather than resolving with a no-op unsubscribe: pbtsdb logs the
 * failure and leaves the collection unsubscribed, which is exactly the intent.
 * Resolving would let it record the collection as subscribed and never retry,
 * so re-enabling realtime later would silently do nothing.
 */
export function installRealtimeGuard() {
    const realtime = pb.realtime as unknown as {
        subscribe: (...args: unknown[]) => Promise<unknown>
        __tinycldGuarded?: boolean
    }
    if (realtime.__tinycldGuarded) return
    const original = realtime.subscribe.bind(realtime)
    realtime.subscribe = (...args: unknown[]) => {
        if (!realtimeEnabled) {
            return Promise.reject(new Error('realtime is disabled for this page'))
        }
        return original(...args)
    }
    realtime.__tinycldGuarded = true
}
