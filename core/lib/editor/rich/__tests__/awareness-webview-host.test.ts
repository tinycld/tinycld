import { describe, expect, it } from 'vitest'
import {
    Awareness,
    applyAwarenessUpdate,
    encodeAwarenessUpdate,
    removeAwarenessStates,
} from 'y-protocols/awareness'
import * as Y from 'yjs'
import type { EditorMessage } from '../../message-bus/types'
import { makeMessage } from '../../message-bus/types'
import { AwarenessWebViewHost } from '../awareness-webview-host'
import {
    AWARENESS_CURSOR,
    AWARENESS_LEAVE,
    AWARENESS_PEERS,
    type AwarenessLeavePayload,
    type AwarenessPeersPayload,
    decodeUpdate,
} from '../webview/source/protocol'

// Carets on native ride this relay. The invariant it exists to protect is "one
// human, one avatar": a phone has two Awareness instances for one person, and
// naively bridging them makes that person two peers. These tests drive both
// sides through the real encode/decode helpers, as the Yjs relay tests do — a
// stub passing Uint8Arrays through would skip the part most likely to break.

const IDENTITY = { id: 'u1', name: 'Ada', color: '#ff0000' }

// The shape y-tiptap keeps in its awareness slot: a pair of relative positions
// as `Y.relativePositionToJSON` emits them. They name items in the DOCUMENT, so
// they resolve in any replica without translation — which is what lets a cursor
// authored inside the WebView land correctly on a web peer.
const CURSOR = {
    anchor: { type: null, tname: 'card:abc', item: { client: 1, clock: 0 }, assoc: 0 },
    head: { type: null, tname: 'card:abc', item: { client: 1, clock: 3 }, assoc: 0 },
}

/** Stands in for the page: its own Awareness, wired to the host by hand. */
function makePage() {
    const doc = new Y.Doc()
    const awareness = new Awareness(doc)
    const FROM_HOST = Symbol('from-host')
    return {
        awareness,
        /** Apply a host→page message the way the page's relay hook does. */
        receive(message: EditorMessage) {
            if (message.type === AWARENESS_PEERS) {
                const { update } = message.payload as AwarenessPeersPayload
                applyAwarenessUpdate(awareness, decodeUpdate(update), FROM_HOST)
                return
            }
            const { clientIDs } = message.payload as AwarenessLeavePayload
            removeAwarenessStates(awareness, clientIDs, FROM_HOST)
        },
        /** The page's own cursor, on its way to the host. */
        cursorMessage(cursor: unknown) {
            return makeMessage('awareness', AWARENESS_CURSOR, { cursor })
        },
        /**
         * The carets the page would DRAW: everyone it knows about except itself.
         *
         * `new Awareness(doc)` registers a local slot immediately, so the page
         * always holds its own — y-tiptap filters it out at render time. What
         * must never appear here is a SECOND slot for the same human, relayed
         * back from the host.
         */
        remoteClientIDs() {
            return [...awareness.getStates().keys()].filter(id => id !== awareness.clientID)
        },
    }
}

function makeHost(awareness: Awareness) {
    const sent: EditorMessage[] = []
    const host = new AwarenessWebViewHost({
        awareness,
        postMessage: message => {
            sent.push(message)
            return true
        },
    })
    return { host, sent }
}

/** A peer on the room socket, as the host's awareness sees it. */
function joinPeer(hostAwareness: Awareness, user: { id: string; name: string; color: string }) {
    const peer = new Awareness(new Y.Doc())
    peer.setLocalState({ user })
    applyAwarenessUpdate(
        hostAwareness,
        encodeAwarenessUpdate(peer, [peer.clientID]),
        'from-network'
    )
    return peer
}

describe('AwarenessWebViewHost', () => {
    it("merges the page's cursor into the host's own slot", () => {
        const hostAwareness = new Awareness(new Y.Doc())
        // Board presence writes here first; the merge must not erase it.
        hostAwareness.setLocalState({ user: IDENTITY, cardId: 'card-1' })
        const { host } = makeHost(hostAwareness)
        const page = makePage()

        host.handleMessage(page.cursorMessage(CURSOR))

        expect(hostAwareness.getLocalState()).toEqual({
            user: IDENTITY,
            cardId: 'card-1',
            cursor: CURSOR,
        })
    })

    it('never opens a second slot for the local user', () => {
        // The whole point of the design: the page's clientID stays inside the
        // WebView, so peers see one avatar and one caret for one human.
        const hostAwareness = new Awareness(new Y.Doc())
        hostAwareness.setLocalState({ user: IDENTITY })
        const { host } = makeHost(hostAwareness)
        const page = makePage()

        host.handleMessage(page.cursorMessage(CURSOR))

        expect(hostAwareness.getStates().size).toBe(1)
        expect([...hostAwareness.getStates().keys()]).toEqual([hostAwareness.clientID])
    })

    it("does not relay the host's own slot to the page", () => {
        // The ghost-caret guard. The local slot carries the cursor the page just
        // sent; relayed back it would arrive under a clientID that is not the
        // page's own, so y-tiptap would happily draw the user their own caret.
        const hostAwareness = new Awareness(new Y.Doc())
        const { sent } = makeHost(hostAwareness)

        hostAwareness.setLocalState({ user: IDENTITY, cursor: CURSOR })

        expect(sent).toEqual([])
    })

    it("relays a remote peer's state to the page", () => {
        const hostAwareness = new Awareness(new Y.Doc())
        const { sent } = makeHost(hostAwareness)
        const page = makePage()

        const peer = joinPeer(hostAwareness, { id: 'u2', name: 'Grace', color: '#00ff00' })
        for (const message of sent) page.receive(message)

        expect(page.awareness.getStates().get(peer.clientID)).toEqual({
            user: { id: 'u2', name: 'Grace', color: '#00ff00' },
        })
    })

    it('tells the page when a peer leaves', () => {
        const hostAwareness = new Awareness(new Y.Doc())
        const { sent } = makeHost(hostAwareness)
        const page = makePage()
        const peer = joinPeer(hostAwareness, { id: 'u2', name: 'Grace', color: '#00ff00' })
        for (const message of sent) page.receive(message)
        sent.length = 0

        removeAwarenessStates(hostAwareness, [peer.clientID], 'from-network')
        for (const message of sent) page.receive(message)

        expect(page.awareness.getStates().has(peer.clientID)).toBe(false)
        // A departed client is gone from `meta` too, and encoding one throws —
        // hence the bare-ids message rather than an encoded update.
        expect(sent.map(m => m.type)).toEqual([AWARENESS_LEAVE])
    })

    it('does not echo the cursor it just applied from the page', () => {
        // Writing the cursor fires a host awareness update naming the local
        // client. If the outbound filter missed it, this would loop forever.
        const hostAwareness = new Awareness(new Y.Doc())
        hostAwareness.setLocalState({ user: IDENTITY })
        const { host, sent } = makeHost(hostAwareness)
        const page = makePage()

        host.handleMessage(page.cursorMessage(CURSOR))

        expect(sent).toEqual([])
    })

    it('survives a peer that left before its state could be encoded', () => {
        // encodeAwarenessUpdate reads `meta.get(id).clock` with no guard, so an
        // id with no meta entry is a TypeError rather than a no-op.
        const hostAwareness = new Awareness(new Y.Doc())
        const { sent } = makeHost(hostAwareness)
        const absent = hostAwareness.clientID + 1

        expect(() =>
            hostAwareness.emit('update', [{ added: [absent], updated: [], removed: [] }, 'test'])
        ).not.toThrow()
        expect(sent).toEqual([])
    })

    it('passes a null cursor through, since that is how blur is signalled', () => {
        const hostAwareness = new Awareness(new Y.Doc())
        hostAwareness.setLocalState({ user: IDENTITY, cursor: CURSOR })
        const { host } = makeHost(hostAwareness)
        const page = makePage()

        host.handleMessage(page.cursorMessage(null))

        // The caret goes away; the avatar does not.
        expect(hostAwareness.getLocalState()).toEqual({ user: IDENTITY, cursor: null })
    })

    it('ignores a malformed cursor rather than poisoning the presence slot', () => {
        const hostAwareness = new Awareness(new Y.Doc())
        hostAwareness.setLocalState({ user: IDENTITY })
        const { host } = makeHost(hostAwareness)
        const page = makePage()

        for (const bad of ['nonsense', 42, {}, { anchor: null, head: null }]) {
            expect(host.handleMessage(page.cursorMessage(bad))).toBe(true)
        }

        expect(hostAwareness.getLocalState()).toEqual({ user: IDENTITY })
    })

    it('seeds the page with the peers already in the room', () => {
        // Awareness frames are only fanned out as they are sent, so without this
        // a phone joining an occupied board sees no carets until someone moves.
        const hostAwareness = new Awareness(new Y.Doc())
        hostAwareness.setLocalState({ user: IDENTITY })
        const { host } = makeHost(hostAwareness)
        const peer = joinPeer(hostAwareness, { id: 'u2', name: 'Grace', color: '#00ff00' })

        const page = makePage()
        const encoded = host.encodePeers()
        expect(encoded).not.toBeNull()
        page.receive(makeMessage('awareness', AWARENESS_PEERS, { update: encoded }))

        expect(page.remoteClientIDs()).toEqual([peer.clientID])
    })

    it('seeds nothing when this client is alone', () => {
        const hostAwareness = new Awareness(new Y.Doc())
        hostAwareness.setLocalState({ user: IDENTITY })
        const { host } = makeHost(hostAwareness)

        expect(host.encodePeers()).toBeNull()
    })

    it('consumes only its own namespace', () => {
        const hostAwareness = new Awareness(new Y.Doc())
        const { host } = makeHost(hostAwareness)

        expect(host.handleMessage(makeMessage('yjs', 'update', { update: 'x' }))).toBe(false)
        expect(host.handleMessage(makeMessage('awareness', 'unknown', {}))).toBe(true)
    })

    it('stops relaying once destroyed', () => {
        const hostAwareness = new Awareness(new Y.Doc())
        const { host, sent } = makeHost(hostAwareness)

        host.destroy()
        joinPeer(hostAwareness, { id: 'u2', name: 'Grace', color: '#00ff00' })

        expect(sent).toEqual([])
    })

    it('converges three peers with exactly one slot for the phone', () => {
        // Web peer + phone host + the phone's WebView page. This is the test
        // that would have caught the bug: the web peer must see ONE state for
        // the phone carrying both its avatar identity and its caret, and the
        // page must never see a slot for its own user.
        const hostAwareness = new Awareness(new Y.Doc())
        hostAwareness.setLocalState({ user: IDENTITY, cardId: 'card-1' })
        const { host, sent } = makeHost(hostAwareness)
        const page = makePage()

        const webPeer = joinPeer(hostAwareness, { id: 'u2', name: 'Grace', color: '#00ff00' })
        for (const message of sent) page.receive(message)

        // The phone user puts a caret in the document, inside the WebView.
        host.handleMessage(page.cursorMessage(CURSOR))

        // What the realtime client would put on the wire: the local slot only.
        applyAwarenessUpdate(
            webPeer,
            encodeAwarenessUpdate(hostAwareness, [hostAwareness.clientID]),
            'from-network'
        )

        const phoneAsSeenByWeb = [...webPeer.getStates().entries()].filter(
            ([, state]) => (state as { user?: { id: string } }).user?.id === IDENTITY.id
        )
        expect(phoneAsSeenByWeb).toHaveLength(1)
        expect(phoneAsSeenByWeb[0][1]).toEqual({
            user: IDENTITY,
            cardId: 'card-1',
            cursor: CURSOR,
        })

        // And the phone sees only the web peer's caret, never a relayed copy of
        // its own — which is exactly what a missing outbound filter would cause.
        expect(page.remoteClientIDs()).toEqual([webPeer.clientID])
    })
})
