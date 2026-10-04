import { RealtimeClient } from '@tinycld/core/lib/realtime/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { Awareness } from 'y-protocols/awareness'
import * as Y from 'yjs'

/**
 * A server in read-only mode refuses the websocket upgrade (503) and closes
 * the connections that were open when the mode started. The client must keep
 * reconnecting with backoff, and an edit made while it is disconnected must
 * reach the server once a connection is back: the server never saw it, and
 * the resync only sends the server's state to the client, not the reverse.
 */

const CLIENT_ID_LEN = 16
const FRAME_OVERHEAD = CLIENT_ID_LEN + 1
const MSG_DOC_UPDATE = 0x01
const MSG_SYNC_REPLY = 0x04
const MSG_ASSIGN_ID = 0x05

class FakeSocket {
    static OPEN = 1
    readyState = 1
    sent: Uint8Array[] = []
    onopen: (() => void) | null = null
    onmessage: ((evt: { data: ArrayBuffer }) => void) | null = null
    onclose: (() => void) | null = null
    onerror: (() => void) | null = null
    binaryType = 'arraybuffer'

    send(frame: Uint8Array) {
        this.sent.push(new Uint8Array(frame))
    }
    close() {}
    deliver(msgType: number, payload: Uint8Array, senderID?: Uint8Array) {
        const frame = new Uint8Array(FRAME_OVERHEAD + payload.length)
        if (senderID) frame.set(senderID, 0)
        frame[CLIENT_ID_LEN] = msgType
        frame.set(payload, FRAME_OVERHEAD)
        this.onmessage?.({ data: frame.buffer as ArrayBuffer })
    }
    /** The browser's view of a refused upgrade: error, then close, never open. */
    refuse() {
        this.onerror?.()
        this.onclose?.()
    }
    /** Open, take a server-assigned id, and finish the sync handshake. */
    connect() {
        this.onopen?.()
        this.deliver(MSG_ASSIGN_ID, new Uint8Array(0), new Uint8Array(CLIENT_ID_LEN).fill(7))
        // An empty reply: the server has no state for this room yet.
        this.deliver(MSG_SYNC_REPLY, new Uint8Array(0))
    }
    docUpdates(): Uint8Array[] {
        return this.sent
            .filter(f => f[CLIENT_ID_LEN] === MSG_DOC_UPDATE)
            .map(f => f.subarray(FRAME_OVERHEAD))
    }
}

let sockets: FakeSocket[] = []

beforeEach(() => {
    vi.useFakeTimers()
    sockets = []
    const WebSocketStub = function WebSocketStub() {
        const socket = new FakeSocket()
        sockets.push(socket)
        return socket
    } as unknown as typeof WebSocket
    ;(WebSocketStub as { OPEN: number }).OPEN = FakeSocket.OPEN
    vi.stubGlobal('WebSocket', WebSocketStub)
})

afterEach(() => {
    vi.useRealTimers()
})

function newClient() {
    const doc = new Y.Doc()
    const awareness = new Awareness(doc)
    const client = new RealtimeClient({ url: 'ws://test/room', doc, awareness })
    client.connect()
    return { doc, awareness, client }
}

describe('RealtimeClient — reconnect', () => {
    it('retries a refused upgrade with a growing backoff', () => {
        const { client } = newClient()

        sockets[0].refuse()
        vi.advanceTimersByTime(500)
        expect(sockets).toHaveLength(2)

        sockets[1].refuse()
        vi.advanceTimersByTime(999)
        expect(sockets).toHaveLength(2)
        vi.advanceTimersByTime(1)
        expect(sockets).toHaveLength(3)

        client.destroy()
    })

    it('reconnects after the server closes an open connection', () => {
        const { client } = newClient()
        sockets[0].connect()

        sockets[0].onclose?.()
        vi.advanceTimersByTime(500)
        expect(sockets).toHaveLength(2)

        client.destroy()
    })

    it('sends an edit made while disconnected once a reconnect resyncs, across refused attempts', () => {
        const { doc, client } = newClient()
        sockets[0].connect()
        sockets[0].onclose?.()

        doc.getText('body').insert(0, 'typed while away')

        vi.advanceTimersByTime(500)
        sockets[1].refuse()
        vi.advanceTimersByTime(1000)
        const socket = sockets[2]
        socket.onopen?.()
        socket.deliver(MSG_ASSIGN_ID, new Uint8Array(0), new Uint8Array(CLIENT_ID_LEN).fill(9))
        // Held until the resync: a server that rebuilt its document reports
        // that in the hello before the sync reply, and the edit must not
        // reach a document it does not belong to.
        expect(socket.docUpdates()).toHaveLength(0)

        socket.deliver(MSG_SYNC_REPLY, new Uint8Array(0))

        const server = new Y.Doc()
        for (const update of socket.docUpdates()) Y.applyUpdate(server, update)
        expect(server.getText('body').toString()).toBe('typed while away')

        client.destroy()
    })

    it('sends nothing after it is destroyed, even for frames already queued', () => {
        const { doc, client } = newClient()
        sockets[0].onopen?.()
        sockets[0].deliver(MSG_ASSIGN_ID, new Uint8Array(0), new Uint8Array(CLIENT_ID_LEN).fill(7))
        doc.getText('body').insert(0, 'stale')

        client.destroy()
        sockets[0].deliver(MSG_SYNC_REPLY, new Uint8Array(0))

        expect(sockets[0].docUpdates()).toHaveLength(0)
    })
})
