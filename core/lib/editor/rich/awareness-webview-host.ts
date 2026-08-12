import { type Awareness, encodeAwarenessUpdate } from 'y-protocols/awareness'
import { type EditorMessage, makeMessage } from '../message-bus/types'
import {
    AWARENESS_CURSOR,
    AWARENESS_LEAVE,
    AWARENESS_PEERS,
    type AwarenessCursorPayload,
    encodeUpdate,
} from './webview/source/protocol'

/**
 * Host side of the awareness relay: carries collaborator carets between the
 * room's Awareness and the editor running inside the WebView.
 *
 * This is what makes remote carets visible on native. The page has its own
 * `Awareness` (it must — Yjs forbids two docs sharing a clientID), so one human
 * on a phone has two awareness instances. Wire the second one to the network and
 * they become two peers: two avatars, two carets, and a slot the host's presence
 * teardown never cleans up. That was the reason carets were left out entirely.
 *
 * The relay avoids it by never letting the page's clientID reach the wire:
 *
 *   - INBOUND (page → host), the page sends its cursor POSITION, not an encoded
 *     awareness state. The host merges it into its OWN local slot. That matters
 *     because the realtime client only ever encodes its own slot — see
 *     `realtime/client.ts`, which bails unless the change touches `clientID` and
 *     then sends `encodeAwarenessUpdate(awareness, [localID])`. A second slot
 *     would never reach a peer, and the broker's one-awareness-id-per-connection
 *     handshake means its leave frame would never be sent either.
 *
 *   - OUTBOUND (host → page), only REMOTE peers' states cross. The host's own
 *     slot is filtered out, because it now carries the cursor the page just
 *     sent: relayed back, it would arrive under a clientID that is not the
 *     page's own, so y-tiptap's "don't draw my own caret" check would not catch
 *     it and the user would watch a ghost caret trail their own typing.
 *
 * A cursor survives the trip untranslated because a Yjs relative position names
 * ITEMS IN THE DOCUMENT, and those ids are identical in every replica.
 *
 * No origin symbol is needed here, unlike the Yjs relay. The merge above fires a
 * host awareness update, but it touches only the local clientID — which the
 * outbound filter already drops — so the echo cannot form. Pinned by a test
 * rather than left to be rediscovered.
 */

type PostMessage = (message: EditorMessage) => boolean

export interface AwarenessWebViewHostOptions {
    /** The room's Awareness. Never construct one for this — pass the live one. */
    awareness: Awareness
    postMessage: PostMessage
}

/** A cursor as it crosses the bridge: opaque JSON, resolved only by peers. */
type RelayedCursor = AwarenessCursorPayload['cursor']

export class AwarenessWebViewHost {
    private readonly awareness: Awareness
    private readonly postMessage: PostMessage
    private readonly onAwarenessUpdate: (
        changes: { added: number[]; updated: number[]; removed: number[] },
        origin: unknown
    ) => void
    private destroyed = false

    constructor(options: AwarenessWebViewHostOptions) {
        this.awareness = options.awareness
        this.postMessage = options.postMessage

        this.onAwarenessUpdate = ({ added, updated, removed }) => {
            if (this.destroyed) return

            const present = [...added, ...updated].filter(id => this.isRelayable(id))
            if (present.length > 0) {
                this.postMessage(
                    makeMessage('awareness', AWARENESS_PEERS, {
                        update: encodeUpdate(encodeAwarenessUpdate(this.awareness, present)),
                    })
                )
            }

            // Departures travel as bare ids: a client that left is usually gone
            // from `meta` too, and encodeAwarenessUpdate reads its clock without
            // a guard, so encoding one throws.
            const gone = removed.filter(id => id !== this.awareness.clientID)
            if (gone.length > 0) {
                this.postMessage(makeMessage('awareness', AWARENESS_LEAVE, { clientIDs: gone }))
            }
        }
        this.awareness.on('update', this.onAwarenessUpdate)
    }

    /**
     * Every remote peer's state, for the page to start from. Null when this
     * client is alone, so the init payload carries no meaningless base64.
     */
    encodePeers(): string | null {
        const remote = [...this.awareness.getStates().keys()].filter(id => this.isRelayable(id))
        if (remote.length === 0) return null
        return encodeUpdate(encodeAwarenessUpdate(this.awareness, remote))
    }

    /**
     * Feed a WebView message in. Returns true if this host consumed it, so
     * callers can stop routing it further.
     */
    handleMessage(message: EditorMessage): boolean {
        if (message.namespace !== 'awareness') return false
        if (message.type !== AWARENESS_CURSOR) return true
        if (this.destroyed) return true

        const payload = message.payload as AwarenessCursorPayload | undefined
        if (!payload || !('cursor' in payload)) return true
        const cursor = normalizeCursor(payload.cursor)
        if (cursor === undefined) return true

        // A merge, not a replace. This slot also carries the `user` and `cardId`
        // that board presence publishes, and a wholesale write would drop the
        // local user out of every peer's avatar row.
        this.awareness.setLocalStateField('cursor', cursor)
        return true
    }

    destroy(): void {
        this.destroyed = true
        this.awareness.off('update', this.onAwarenessUpdate)
    }

    /**
     * Whether a client's state may cross to the page.
     *
     * Two conditions, both load-bearing: it must not be the local slot (the
     * ghost-caret guard described above), and it must still have a `meta` entry,
     * or `encodeAwarenessUpdate` throws reading its clock.
     */
    private isRelayable(clientID: number): boolean {
        return clientID !== this.awareness.clientID && this.awareness.meta.has(clientID)
    }
}

/**
 * Validate a cursor arriving from the page.
 *
 * Returns the value to store, or `undefined` when the payload is malformed and
 * should be ignored. `null` is meaningful and must pass through — y-tiptap
 * writes it on blur, and dropping it would leave the phone's caret frozen on
 * every peer's screen after the user stopped editing.
 *
 * Worth checking because this slot is shared with presence: a malformed write
 * lands next to `user`, and presence parsing drops the whole slot — taking the
 * avatar with it.
 */
function normalizeCursor(cursor: unknown): RelayedCursor | undefined {
    if (cursor === null) return null
    if (typeof cursor !== 'object') return undefined
    const candidate = cursor as { anchor?: unknown; head?: unknown }
    if (candidate.anchor == null || candidate.head == null) return undefined
    return { anchor: candidate.anchor, head: candidate.head }
}
