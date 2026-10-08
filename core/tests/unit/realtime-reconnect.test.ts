import { RealtimeClient } from '@tinycld/core/lib/realtime/client'
import * as encoding from 'lib0/encoding'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { Awareness } from 'y-protocols/awareness'
import * as Y from 'yjs'

/**
 * A server that restarts, pauses for an update, or drains closes the
 * connection and refuses the upgrade for a while. The client must keep
 * reconnecting with backoff, and once a connection is back it must send the
 * server whatever the server lacks: the server's sync reply carries only ITS
 * state, so an edit made while disconnected would otherwise stay on this
 * client forever. The reply names the server's state vector, and the client
 * answers with the diff against it.
 */

const CLIENT_ID_LEN = 16
const FRAME_OVERHEAD = CLIENT_ID_LEN + 1
const MSG_DOC_UPDATE = 0x01
const MSG_SYNC_REPLY = 0x04
const MSG_ASSIGN_ID = 0x05
const SYNC_STEP2 = 1

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
    /** Open and take a server-assigned id. */
    open() {
        this.onopen?.()
        this.deliver(MSG_ASSIGN_ID, new Uint8Array(0), new Uint8Array(CLIENT_ID_LEN).fill(7))
    }
    /** The server's sync reply: its full state in a y-protocols step2 envelope. */
    syncReply(server: Y.Doc) {
        const enc = encoding.createEncoder()
        encoding.writeVarUint(enc, SYNC_STEP2)
        encoding.writeVarUint8Array(enc, Y.encodeStateAsUpdate(server))
        this.deliver(MSG_SYNC_REPLY, encoding.toUint8Array(enc))
    }
    docUpdates(): Uint8Array[] {
        return this.sent
            .filter(f => f[CLIENT_ID_LEN] === MSG_DOC_UPDATE)
            .map(f => f.subarray(FRAME_OVERHEAD))
    }
}

let sockets: FakeSocket[] = []
let connects: { url: string; protocols: string[] }[] = []

beforeEach(() => {
    vi.useFakeTimers()
    sockets = []
    connects = []
    const WebSocketStub = function WebSocketStub(url: string, protocols: string[]) {
        connects.push({ url, protocols })
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

/** A server document holding `text`, the state a room serves after a restart. */
function serverWith(text: string) {
    const server = new Y.Doc()
    server.getText('body').insert(0, text)
    return server
}

/** Apply every update the socket sent to the server and return its text. */
function serverTextAfter(server: Y.Doc, socket: FakeSocket) {
    for (const update of socket.docUpdates()) Y.applyUpdate(server, update)
    return server.getText('body').toString()
}

describe('RealtimeClient — reconnect', () => {
    it('reads the credential afresh on every connect and keeps it out of the URL', () => {
        let token = 'first'
        const doc = new Y.Doc()
        const client = new RealtimeClient({
            url: 'ws://test/room',
            protocols: () => ['tinycld.realtime', `tinycld.auth.${token}`],
            doc,
            awareness: new Awareness(doc),
        })
        client.connect()

        token = 'refreshed'
        sockets[0].refuse()
        vi.advanceTimersByTime(500)

        expect(connects).toEqual([
            { url: 'ws://test/room', protocols: ['tinycld.realtime', 'tinycld.auth.first'] },
            { url: 'ws://test/room', protocols: ['tinycld.realtime', 'tinycld.auth.refreshed'] },
        ])
        client.destroy()
    })

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
        sockets[0].open()
        sockets[0].syncReply(serverWith(''))

        sockets[0].onclose?.()
        vi.advanceTimersByTime(500)
        expect(sockets).toHaveLength(2)

        client.destroy()
    })

    it('sends an edit made while disconnected once a reconnect resyncs, across refused attempts', () => {
        const server = serverWith('base ')
        const { doc, client } = newClient()
        sockets[0].open()
        sockets[0].syncReply(server)
        sockets[0].onclose?.()

        doc.getText('body').insert(doc.getText('body').length, 'typed while away')

        vi.advanceTimersByTime(500)
        sockets[1].refuse()
        vi.advanceTimersByTime(1000)
        const socket = sockets[2]
        socket.open()
        // Held until the resync: a server that rebuilt its document reports
        // that in the hello before the sync reply, and the edit must not
        // reach a document it does not belong to.
        expect(socket.docUpdates()).toHaveLength(0)

        socket.syncReply(server)

        expect(serverTextAfter(server, socket)).toBe('base typed while away')
        client.destroy()
    })

    it('sends an offline deletion', () => {
        const server = serverWith('hello world')
        const { doc, client } = newClient()
        sockets[0].open()
        sockets[0].syncReply(server)
        sockets[0].onclose?.()

        doc.getText('body').delete(5, 6)

        vi.advanceTimersByTime(500)
        const socket = sockets[1]
        socket.open()
        socket.syncReply(server)

        expect(socket.docUpdates().length).toBeGreaterThan(0)
        expect(serverTextAfter(server, socket)).toBe('hello')
        client.destroy()
    })

    it('sends nothing when it is already in sync', () => {
        const server = serverWith('same')
        const { client } = newClient()
        sockets[0].open()
        sockets[0].syncReply(server)
        sockets[0].onclose?.()

        vi.advanceTimersByTime(500)
        const socket = sockets[1]
        socket.open()
        socket.syncReply(server)

        expect(socket.docUpdates()).toHaveLength(0)
        client.destroy()
    })

    it('sends a live edit at once after the sync, not as a second diff', () => {
        const server = serverWith('')
        const { doc, client } = newClient()
        sockets[0].open()
        sockets[0].syncReply(server)

        doc.getText('body').insert(0, 'live')

        expect(sockets[0].docUpdates()).toHaveLength(1)
        expect(serverTextAfter(server, sockets[0])).toBe('live')
        client.destroy()
    })

    it('sends nothing after it is destroyed, even for a reply that arrives late', () => {
        const server = serverWith('')
        const { doc, client } = newClient()
        sockets[0].open()
        doc.getText('body').insert(0, 'stale')

        client.destroy()
        sockets[0].syncReply(server)

        expect(sockets[0].docUpdates()).toHaveLength(0)
    })
})
