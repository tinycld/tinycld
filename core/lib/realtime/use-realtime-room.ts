import { useFocusEffect } from 'expo-router'
import { type RefObject, useCallback, useEffect, useRef, useState } from 'react'
import { Awareness } from 'y-protocols/awareness'
import * as Y from 'yjs'
import { PB_SERVER_ADDR } from '../config'
import { captureException } from '../errors'
import { log } from '../logger'
import { pb } from '../pocketbase'
import { RealtimeClient } from './client'

export interface UseRealtimeRoomOptions {
    // The roomKind name registered on the server side (matches the
    // string passed to realtime.RegisterRoomKind in Go). Determines
    // which authorize handler gates this connection. Empty string
    // disables the room (returns null).
    roomKind: string

    // Opaque identifier for the room within roomKind. For sheets this
    // is the drive_item.id. Empty string disables the room (returns
    // null) — useful while a parent is still loading the id from a
    // query.
    roomID: string

    // Initial awareness state to publish as soon as the connection
    // opens. Consumers fill this with their app-specific shape (user
    // identity, color, view-state). Pass null to leave the slot
    // empty; remote tabs will see only an empty Awareness entry.
    initialAwareness: Record<string, unknown> | null

    // onFirstJoinerBootstrap is called when:
    //   (a) the server's initial SYNC_REPLY says we're alone, OR
    //   (b) the SYNC_REPLY came from a peer but we still look empty
    //       (the peer was a stale ghost or had no state to share).
    //
    // Case (b) protects against rendering an empty doc just because
    // a misbehaving peer answered the handshake first. The consumer
    // decides what "empty" means for its schema via isEmpty.
    //
    // The hook flips isReady=true after the callback resolves (or
    // immediately, if no callback is given).
    onFirstJoinerBootstrap?: (doc: Y.Doc) => Promise<void> | void

    // isEmpty returns true when the doc has no consumer-meaningful
    // state. Used to decide whether onFirstJoinerBootstrap should run
    // even after a peer reply (case (b) above). Defaults to "no
    // top-level shared types defined yet," which is conservative and
    // works for any schema where the consumer's bootstrap creates at
    // least one top-level Y.Map / Y.Array.
    isEmpty?: (doc: Y.Doc) => boolean

    // shareSession, when set, authorizes the connection as an anonymous
    // share-link visitor (no PB auth). The token is attached as
    // ?share_session=; the server's handleConnect verifies it and runs
    // the room kind's AuthorizeShare. When unset, the PB auth token is
    // used as before.
    shareSession?: string

    // docEpochOf reads this room kind's document epoch out of the server
    // hello, or returns null when the payload carries none.
    //
    // A server may DISCARD an idle document and rebuild it from storage —
    // cards' janitor evicts a quiet board, and the next joiner's connect
    // re-seeds the fragments from the cards table. The rebuilt document is a
    // different incarnation: y-crdt mints a fresh clientID for it, so the
    // inserts our surviving Y.Doc still holds are, to the CRDT, edits nobody
    // has seen rather than the same text arriving twice.
    //
    // Merging across that boundary therefore CONVERGES on both copies. That is
    // the CRDT working correctly — there is no merge that could recover the
    // intent, because the two insert sets are genuinely independent — so the
    // only correct move is to throw our state away and resync from scratch.
    // Supplying this opts a room kind into that, and the doc is rebuilt
    // whenever the epoch changes from the one we synced under.
    //
    // Rooms that leave it undefined keep the previous behavior exactly.
    docEpochOf?: (serverHello: unknown) => number | null
}

export interface RealtimeRoomHandle {
    doc: Y.Doc
    awareness: Awareness
    // True after the initial sync handshake (or first-joiner
    // bootstrap) completes. Callers gate their UI on this — render a
    // loading state until then.
    isReady: boolean
    // True while the underlying WebSocket is open. Flips back to
    // false on disconnect and reconnect cycles. Useful for surfacing
    // "reconnecting…" affordances in the UI; orthogonal to isReady,
    // which only flips once.
    isConnected: boolean
    // serverHello carries the payload of the MsgServerHello frame the
    // broker sent on connect, JSON-parsed. Null until the frame
    // arrives. Schema is opaque (defined per room kind); consumers cast
    // to their kind-specific shape. On reconnect, this resets to null
    // and is repopulated when the new connection's hello arrives.
    serverHello: unknown
    // serverSlot carries the latest payload of a MsgServerSlot frame
    // (room-wide server state, e.g. saveStatus). Null until the first
    // frame arrives. Schema is opaque (defined per room kind); consumers
    // cast to their kind-specific shape. Resets to null on disconnect/
    // cleanup; updates on every incoming MsgServerSlot frame.
    serverSlot: unknown
}

// useRealtimeRoom owns one Y.Doc + Awareness pair, opens a WebSocket
// to /api/realtime/<roomKind>/<roomID>, runs the y-protocols/sync
// handshake, and either applies remote state from a peer or invokes
// onFirstJoinerBootstrap to seed an empty doc.
//
// The returned handle is null while the doc/awareness are still being
// constructed (one-frame race with the first effect tick), or while
// roomKind/roomID are empty. Callers should render a loading state
// until both `handle != null` and `handle.isReady`.
//
export function useRealtimeRoom({
    roomKind,
    roomID,
    initialAwareness,
    onFirstJoinerBootstrap,
    isEmpty = defaultIsEmpty,
    shareSession,
    docEpochOf,
}: UseRealtimeRoomOptions): RealtimeRoomHandle | null {
    const [isReady, setIsReady] = useState(false)
    const [isConnected, setIsConnected] = useState(false)
    const [serverHello, setServerHello] = useState<unknown>(null)
    // How many times the server's document has been REPLACED under us.
    //
    // The effect below keys on this rather than on the epoch value, because
    // learning the epoch for the first time must not rebuild anything: a fresh
    // doc has no stale state to discard, and tearing it down would drop the
    // sync that is already in flight. Only a change from one known epoch to a
    // different one is a discard, and each one bumps this by exactly 1.
    const [docGeneration, setDocGeneration] = useState(0)
    // The epoch the live doc was built against, held in a ref so the hello
    // handler can compare without the connection effect depending on it.
    // Written from the handler rather than through state for the same reason:
    // the comparison must see the value the CURRENT doc was created under, not
    // one a pending render has yet to commit.
    const docEpochRef = useRef<number | null>(null)
    // Read through a ref for the same reason the other callbacks are: the
    // caller passes an inline closure, and depending on its identity would
    // reopen the socket on every render.
    const docEpochOfRef = useRef(docEpochOf)
    docEpochOfRef.current = docEpochOf
    const [serverSlot, setServerSlot] = useState<unknown>(null)
    const handleRef = useRef<{ doc: Y.Doc; awareness: Awareness; client: RealtimeClient } | null>(
        null
    )
    // Force a re-render once the doc + awareness are constructed. The
    // ref-only-mutation in the effect would otherwise be invisible to
    // React.
    const [, setBumpKey] = useState(0)

    // biome-ignore lint/correctness/useExhaustiveDependencies: initialAwareness and onFirstJoinerBootstrap are intentionally captured by closure on the first effect run only — we don't want to tear down and re-open the WS every time the caller hands us a new closure reference. roomKind/roomID gate the lifecycle.
    useEffect(() => {
        if (!roomKind || !roomID) return

        const wsURL = buildRealtimeURL(roomKind, roomID, shareSession)

        const doc = new Y.Doc()
        const awareness = new Awareness(doc)
        if (initialAwareness != null) {
            awareness.setLocalState(initialAwareness)
        }

        let cancelled = false

        const client = new RealtimeClient({
            url: wsURL,
            doc,
            awareness,
            onOpen: () => {
                if (!cancelled) setIsConnected(true)
            },
            onClose: () => {
                if (!cancelled) {
                    setIsConnected(false)
                    setServerHello(null)
                    setServerSlot(null)
                }
            },
            onServerHello: payload => {
                if (cancelled) return
                try {
                    const text = new TextDecoder().decode(payload)
                    const parsed = text.length > 0 ? JSON.parse(text) : null
                    setServerHello(parsed)

                    // The epoch check runs BEFORE the sync handshake can fold
                    // remote state into this doc, because the hello frame
                    // precedes the sync reply. That ordering is what makes
                    // discarding cheap: we drop a doc that has not yet been
                    // contaminated, rather than trying to unpick a merge.
                    const epoch = docEpochOfRef.current?.(parsed) ?? null
                    if (epoch != null) {
                        const previous = docEpochRef.current
                        docEpochRef.current = epoch
                        // Learning the epoch for the first time is not a
                        // discard — this doc has nothing stale in it yet.
                        // Reconnecting to the SAME incarnation is not one
                        // either, or every network blip would throw away
                        // unsynced edits. Only a genuine replacement rebuilds.
                        if (previous != null && previous !== epoch) {
                            log.warn(
                                'realtime.docEpoch',
                                'server document was rebuilt; discarding local state',
                                { roomKind, roomID, previous, epoch }
                            )
                            setDocGeneration(n => n + 1)
                        }
                    }
                } catch (err) {
                    captureException('realtime.serverHello.parse', err, {
                        roomKind,
                        roomID,
                    })
                    setServerHello(null)
                }
            },
            onServerSlot: payload => {
                if (cancelled) return
                try {
                    const text = new TextDecoder().decode(payload)
                    const parsed = text.length > 0 ? JSON.parse(text) : null
                    setServerSlot(parsed)
                } catch (err) {
                    captureException('realtime.serverSlot.parse', err, {
                        roomKind,
                        roomID,
                    })
                    setServerSlot(null)
                }
            },
            onSyncReply: async hadPeer => {
                if (cancelled) return
                // Bootstrap when there is no peer at all OR when the
                // peer gave us a reply that left us with no state.
                // The latter happens if the room held a ghost peer
                // (a dead WS that hadn't been cleaned up server-side
                // yet) — falling back to the consumer's bootstrap
                // path keeps us from rendering an empty doc just
                // because we trusted a misbehaving peer.
                const needsBootstrap = !hadPeer || isEmpty(doc)
                if (needsBootstrap && onFirstJoinerBootstrap) {
                    try {
                        await onFirstJoinerBootstrap(doc)
                    } catch (err) {
                        // Surface to Sentry so silent empty-doc
                        // renders don't disappear into the void; the
                        // UI still falls through to its own empty
                        // state, but at least we'll see the cause.
                        captureException('realtime.bootstrap', err, {
                            roomKind,
                            roomID,
                        })
                    }
                }
                if (!cancelled) setIsReady(true)
            },
        })

        handleRef.current = { doc, awareness, client }
        setBumpKey(n => n + 1)
        client.connect()

        return () => {
            cancelled = true
            // Signal a clean leave to peers before tearing down the
            // transport. The awareness null-state emits a removal
            // frame that other clients can drop immediately, instead
            // of waiting for the server's heartbeat-loss broadcast.
            // ref: https://docs.yjs.dev/api/about-awareness
            try {
                awareness.setLocalState(null)
            } catch {
                // best effort — destroy below tears it down regardless
            }
            client.destroy()
            awareness.destroy()
            doc.destroy()
            handleRef.current = null
            // The next doc is a NEW one, so it has no epoch until its own hello
            // names one. Leaving the old value here would make that first hello
            // read as a second replacement and rebuild again, forever.
            docEpochRef.current = null
            setIsReady(false)
            setIsConnected(false)
            setServerHello(null)
            setServerSlot(null)
        }
        // `docGeneration` belongs here: it increments only when the server
        // reports it replaced its document, and this effect's teardown/setup is
        // exactly the discard — the old Y.Doc is destroyed and a fresh one
        // resyncs from scratch. It never moves in an ordinary session, so the
        // common path still opens one socket.
    }, [roomKind, roomID, docGeneration])

    // Publish a clean leave whenever this screen stops being visible, and
    // republish on the way back.
    //
    // Keyed on FOCUS and on pagehide, NOT on unmount alone. The effect
    // cleanup above only runs when the component actually unmounts, and for
    // package-to-package navigation it never does: the package tabs render
    // with `freezeOnBlur` (core/components/workspace/PackageTabs.tsx), which
    // leaves a departed screen mounted-but-frozen so returning to it is
    // instant. Its socket therefore stays open and its awareness slot stays
    // populated, so remote peers kept showing an avatar for someone who had
    // left the package — the bug cards/tests/e2e/board-presence.spec.ts
    // catches. core/lib/shortcuts/scopes.ts solves the same freeze the same
    // way, for the same reason.
    //
    // A tab close is the other half: it never unmounts anything either, and
    // `pagehide` is the one event that fires reliably on mobile Safari and
    // also covers bfcache eviction. Web-only, hence the typeof guard — this
    // module is imported by React Native.
    useLeaveOnBlur(handleRef)

    if (!handleRef.current) return null
    return {
        doc: handleRef.current.doc,
        awareness: handleRef.current.awareness,
        isReady,
        isConnected,
        serverHello,
        serverSlot,
    }
}

// useLeaveOnBlur publishes an awareness removal when the owning route
// blurs or the page hides, and restores the slot on refocus.
//
// The saved slot is what makes this reversible: `setLocalState(null)`
// deletes the local state outright, so without stashing it first a
// refocus would restore an empty slot and the user would be invisible
// to peers until something else happened to republish. Consumers that
// keep their own publish effect (cards' useBoardPresence) would recover
// eventually; ones that publish only via `initialAwareness` would not,
// since that is captured on the room's first effect run only.
function useLeaveOnBlur(
    handleRef: RefObject<{ doc: Y.Doc; awareness: Awareness; client: RealtimeClient } | null>
) {
    const savedSlot = useRef<Record<string, unknown> | null>(null)

    const leave = useCallback(() => {
        const awareness = handleRef.current?.awareness
        if (awareness == null) return
        const current = awareness.getLocalState()
        // Don't clobber a good saved slot with the null we just wrote —
        // pagehide and blur can both fire for one departure.
        if (current != null) savedSlot.current = current as Record<string, unknown>
        try {
            awareness.setLocalState(null)
        } catch {
            // best effort — a destroyed awareness is already "left"
        }
    }, [handleRef])

    const rejoin = useCallback(() => {
        const awareness = handleRef.current?.awareness
        if (awareness == null || savedSlot.current == null) return
        try {
            awareness.setLocalState(savedSlot.current)
        } catch {
            // best effort
        }
        savedSlot.current = null
    }, [handleRef])

    useFocusEffect(
        useCallback(() => {
            // Refocusing a frozen screen remounts nothing, so the slot has
            // to be restored here rather than by the room effect.
            rejoin()
            return leave
        }, [leave, rejoin])
    )

    useEffect(() => {
        // Feature-detect the METHOD, not the object. React Native defines a
        // global `window` with no DOM event API, so a `typeof window` check
        // passes there and then throws "addEventListener is not a function",
        // taking the whole screen down. There is no pagehide on native
        // anyway — the app is backgrounded, not unloaded, and blur already
        // covers navigating away.
        if (typeof window === 'undefined' || typeof window.addEventListener !== 'function') return
        window.addEventListener('pagehide', leave)
        return () => window.removeEventListener('pagehide', leave)
    }, [leave])
}

// defaultIsEmpty considers a Y.Doc empty when no top-level shared
// type has any content. This is a slightly-stronger signal than
// `share.size === 0` — applying a sync reply seeds `share` with the
// type names regardless of whether any content arrived, so we have
// to look inside.
//
// Consumers with unusual schemas (e.g. a Y.Text whose empty state is
// still meaningful) can override via the isEmpty option.
function defaultIsEmpty(doc: Y.Doc): boolean {
    if (doc.share.size === 0) return true
    for (const [, type] of doc.share) {
        if (type instanceof Y.Map && type.size > 0) return false
        if (type instanceof Y.Array && type.length > 0) return false
        if (type instanceof Y.Text && type.length > 0) return false
    }
    return true
}

// PB_SERVER_ADDR is the resolved PocketBase origin set during app
// init — same-origin on web, an explicit http(s):// URL on native
// (from the EXPO_PUBLIC_ENV → serverShortcuts mapping in coreConfig).
// Using it here keeps the realtime WS pointed at the same server as
// every other API call, on every platform.
function buildRealtimeURL(roomKind: string, roomID: string, shareSession?: string): string {
    const httpURL = `${PB_SERVER_ADDR}/api/realtime/${encodeURIComponent(roomKind)}/${encodeURIComponent(roomID)}`
    const wsURL = httpURL.replace(/^http/i, 'ws')
    // Browsers can't set custom headers on a WebSocket upgrade, so we
    // attach the credential as a query param. The server's handleConnect
    // reads ?token= via FindAuthRecordByToken (authenticated user) or
    // ?share_session= via the share-session resolver (anonymous link
    // visitor). React Native's WebSocket has the same header limitation,
    // so the same query approach works on every platform.
    if (shareSession) {
        return `${wsURL}?share_session=${encodeURIComponent(shareSession)}`
    }
    const token = pb.authStore.token
    const tokenQuery = token ? `?token=${encodeURIComponent(token)}` : ''
    return `${wsURL}${tokenQuery}`
}
