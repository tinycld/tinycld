// @vitest-environment happy-dom
import { act, render, waitFor } from '@testing-library/react'
import { useRealtimeRoom } from '@tinycld/core/lib/realtime/use-realtime-room'
import { setResolvedAddress } from '@tinycld/core/lib/server-address'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type * as Y from 'yjs'

/**
 * A server may DISCARD an idle collaborative document and rebuild it from
 * storage — boards' janitor evicts a quiet board, and the next joiner re-seeds
 * every card's description out of the cards table.
 *
 * The rebuilt document is a different incarnation, and y-crdt mints a fresh
 * clientID for it. The inserts our surviving Y.Doc still holds are therefore
 * NOT recognizable as the same text: they are independent operations nobody has
 * seen. Merging converges — correctly, and on BOTH copies — which is what put a
 * card's description on screen doubled, then tripled.
 *
 * `docEpochOf` is how a room opts into discarding local state instead. These
 * tests pin the two halves of that: an unchanged epoch must NOT disturb a live
 * session, and a changed one must rebuild the doc.
 */

const CLIENT_ID_LEN = 16
const FRAME_OVERHEAD = CLIENT_ID_LEN + 1
const MSG_ASSIGN_ID = 0x05
const MSG_SERVER_HELLO = 0x06

class FakeSocket {
    static OPEN = 1
    readyState = 1
    sent: Uint8Array[] = []
    closed = false
    onopen: (() => void) | null = null
    onmessage: ((evt: { data: ArrayBuffer }) => void) | null = null
    onclose: (() => void) | null = null
    onerror: (() => void) | null = null
    binaryType = 'arraybuffer'

    send(frame: Uint8Array) {
        this.sent.push(new Uint8Array(frame))
    }
    close() {
        this.closed = true
    }
    deliver(msgType: number, payload: Uint8Array, senderID?: Uint8Array) {
        const frame = new Uint8Array(FRAME_OVERHEAD + payload.length)
        if (senderID) frame.set(senderID, 0)
        frame[CLIENT_ID_LEN] = msgType
        frame.set(payload, FRAME_OVERHEAD)
        this.onmessage?.({ data: frame.buffer as ArrayBuffer })
    }
    /** The server's per-connection handshake payload, as JSON bytes. */
    deliverHello(payload: unknown) {
        this.deliver(MSG_SERVER_HELLO, new TextEncoder().encode(JSON.stringify(payload)))
    }
}

let sockets: FakeSocket[] = []

afterEach(() => setResolvedAddress(null))

beforeEach(() => {
    // buildRealtimeURL reads the deployment address, which the app resolves
    // through its layout gate rather than a constant.
    setResolvedAddress('https://test.tinycld.org')
    sockets = []
    const WebSocketStub = function WebSocketStub() {
        const socket = new FakeSocket()
        sockets.push(socket)
        return socket
    } as unknown as typeof WebSocket
    ;(WebSocketStub as { OPEN: number }).OPEN = FakeSocket.OPEN
    vi.stubGlobal('WebSocket', WebSocketStub)
})

interface Captured {
    doc: Y.Doc | null
}

function Harness({ captured }: { captured: Captured }) {
    const room = useRealtimeRoom({
        roomKind: 'boards',
        roomID: 'board-1',
        initialAwareness: null,
        docEpochOf: hello =>
            typeof (hello as { docEpoch?: unknown })?.docEpoch === 'number'
                ? (hello as { docEpoch: number }).docEpoch
                : null,
    })
    captured.doc = room?.doc ?? null
    return null
}

/** Bring a socket up far enough that the hook will accept a hello. */
function open(socket: FakeSocket) {
    socket.onopen?.()
    socket.deliver(MSG_ASSIGN_ID, new Uint8Array(0), new Uint8Array(CLIENT_ID_LEN).fill(7))
}

describe('useRealtimeRoom — document epoch', () => {
    it('keeps the same document while the epoch is unchanged', async () => {
        const captured: Captured = { doc: null }
        render(<Harness captured={captured} />)
        await waitFor(() => expect(captured.doc).not.toBeNull())

        const first = captured.doc
        act(() => {
            open(sockets[0])
            sockets[0].deliverHello({ readOnly: false, docEpoch: 42 })
        })
        // A reconnect to the SAME incarnation: the local state is still valid,
        // and tearing the doc down here would throw away unsynced edits on
        // every ordinary network blip.
        act(() => {
            sockets[0].deliverHello({ readOnly: false, docEpoch: 42 })
        })

        await waitFor(() => expect(captured.doc).toBe(first))
        expect(sockets).toHaveLength(1)
    })

    it('rebuilds the document when the server reports a new epoch', async () => {
        const captured: Captured = { doc: null }
        render(<Harness captured={captured} />)
        await waitFor(() => expect(captured.doc).not.toBeNull())

        act(() => {
            open(sockets[0])
            sockets[0].deliverHello({ readOnly: false, docEpoch: 1 })
        })
        const first = captured.doc
        // Something only the local doc knows, so the assertion below proves the
        // state was DISCARDED rather than merely that the object changed.
        act(() => {
            first?.getText('probe').insert(0, 'stale')
        })

        // The board was evicted and rebuilt while we were away. Everything our
        // doc holds predates that incarnation and must not reach it.
        act(() => {
            sockets[0].deliverHello({ readOnly: false, docEpoch: 2 })
        })

        await waitFor(() => expect(captured.doc).not.toBe(first))
        expect(captured.doc?.getText('probe').toString()).toBe('')
        // The discard is a reconnect: the old socket is torn down and a fresh
        // one resyncs from the server.
        expect(sockets.length).toBeGreaterThan(1)
    })

    it('leaves a room that reports no epoch alone', async () => {
        const captured: Captured = { doc: null }
        render(<Harness captured={captured} />)
        await waitFor(() => expect(captured.doc).not.toBeNull())

        const first = captured.doc
        act(() => {
            open(sockets[0])
            // A room kind whose hello carries no epoch at all — text and calc
            // today. Nothing about their behavior may change.
            sockets[0].deliverHello({ readOnly: false })
        })

        await waitFor(() => expect(captured.doc).toBe(first))
        expect(sockets).toHaveLength(1)
    })
})
