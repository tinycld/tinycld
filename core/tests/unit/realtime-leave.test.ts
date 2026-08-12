import { RealtimeClient } from '@tinycld/core/lib/realtime/client'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { Awareness, applyAwarenessUpdate, encodeAwarenessUpdate } from 'y-protocols/awareness'
import * as Y from 'yjs'

// Wire constants, mirrored from client.ts (which keeps them private).
const CLIENT_ID_LEN = 16
const FRAME_OVERHEAD = CLIENT_ID_LEN + 1
const MSG_AWARENESS_UPDATE = 0x02
const MSG_SYNC_REQUEST = 0x03
const MSG_ASSIGN_ID = 0x05
const MSG_AWARENESS_HELLO = 0x08

// A stand-in for the browser WebSocket that records what the client
// sends and lets a test push frames back. readyState must report OPEN or
// the client's sendNow drops every frame on the floor.
class FakeSocket {
    static OPEN = 1
    readyState = 1
    sent: Uint8Array[] = []
    closed = false
    // Order matters for the teardown test: it asserts the removal frame
    // was written BEFORE the socket closed.
    log: string[] = []
    onopen: (() => void) | null = null
    onmessage: ((evt: { data: ArrayBuffer }) => void) | null = null
    onclose: (() => void) | null = null
    onerror: (() => void) | null = null
    binaryType = 'arraybuffer'

    send(frame: Uint8Array) {
        this.sent.push(new Uint8Array(frame))
        this.log.push('send')
    }
    close() {
        this.closed = true
        this.log.push('close')
    }
    /** Deliver a server frame to the client under test. */
    deliver(msgType: number, payload: Uint8Array, senderID?: Uint8Array) {
        const frame = new Uint8Array(FRAME_OVERHEAD + payload.length)
        if (senderID) frame.set(senderID, 0)
        frame[CLIENT_ID_LEN] = msgType
        frame.set(payload, FRAME_OVERHEAD)
        this.onmessage?.({ data: frame.buffer as ArrayBuffer })
    }
}

let sockets: FakeSocket[] = []

beforeEach(() => {
    sockets = []
    // A plain function stands in for the WebSocket constructor: `new Fn()`
    // on a function returning an object yields that object, so each client
    // gets a FakeSocket we can still reach through `sockets`.
    const WebSocketStub = function WebSocketStub() {
        const socket = new FakeSocket()
        sockets.push(socket)
        return socket
    } as unknown as typeof WebSocket
    ;(WebSocketStub as { OPEN: number }).OPEN = FakeSocket.OPEN
    vi.stubGlobal('WebSocket', WebSocketStub)
})

function frameType(frame: Uint8Array): number {
    return frame[CLIENT_ID_LEN]
}

function framePayload(frame: Uint8Array): Uint8Array {
    return frame.subarray(FRAME_OVERHEAD)
}

/** Decode a lib0 varuint, the payload shape of MSG_AWARENESS_HELLO. */
function readVarUint(bytes: Uint8Array): number {
    let value = 0
    let shift = 0
    for (const byte of bytes) {
        value |= (byte & 0x7f) << shift
        if ((byte & 0x80) === 0) return value >>> 0
        shift += 7
    }
    throw new Error('truncated varuint')
}

function connected() {
    const doc = new Y.Doc()
    const awareness = new Awareness(doc)
    const client = new RealtimeClient({ url: 'ws://test/room', doc, awareness })
    client.connect()
    const ws = sockets[0]
    ws.onopen?.()
    // The server assigns an id; its 16-byte prefix IS the assignment.
    const assigned = new Uint8Array(CLIENT_ID_LEN).fill(7)
    ws.deliver(MSG_ASSIGN_ID, new Uint8Array(0), assigned)
    return { doc, awareness, client, ws }
}

describe('RealtimeClient — awareness hello', () => {
    it('announces its yjs clientID as soon as the server assigns an id', () => {
        const { awareness, ws, client } = connected()

        const hello = ws.sent.find(f => frameType(f) === MSG_AWARENESS_HELLO)
        expect(hello, 'no MSG_AWARENESS_HELLO was sent').toBeDefined()
        // The broker keys the synthesized leave frame off this value, so a
        // mismatch means peers would be told to drop the wrong avatar.
        expect(readVarUint(framePayload(hello as Uint8Array))).toBe(awareness.clientID)

        client.destroy()
    })

    it('announces before the sync handshake, so the broker knows the slot first', () => {
        const { ws, client } = connected()

        const helloAt = ws.sent.findIndex(f => frameType(f) === MSG_AWARENESS_HELLO)
        const syncAt = ws.sent.findIndex(f => frameType(f) === MSG_SYNC_REQUEST)
        expect(helloAt).toBeGreaterThanOrEqual(0)
        expect(syncAt).toBeGreaterThanOrEqual(0)
        expect(helloAt).toBeLessThan(syncAt)

        client.destroy()
    })
})

describe('RealtimeClient — leave frames', () => {
    it('drops a peer slot when the broker synthesizes a real removal', () => {
        const { awareness, ws, client } = connected()

        // Seed a peer slot the way a real awareness update would.
        const peerDoc = new Y.Doc()
        const peer = new Awareness(peerDoc)
        peer.setLocalState({ user: { id: 'ghost', name: 'Ghost', color: '#fff' } })
        ws.deliver(MSG_AWARENESS_UPDATE, encodeAwarenessUpdate(peer, [peer.clientID]))
        expect(awareness.getStates().has(peer.clientID)).toBe(true)

        // The broker's synthesized removal: the peer left ungracefully, so
        // this frame is the only notice anyone gets.
        peer.setLocalState(null)
        ws.deliver(MSG_AWARENESS_UPDATE, encodeAwarenessUpdate(peer, [peer.clientID]))

        expect(awareness.getStates().has(peer.clientID)).toBe(false)

        client.destroy()
        peer.destroy()
        peerDoc.destroy()
    })

    it('ignores a legacy zero-length leave frame without throwing', () => {
        const { awareness, ws, client } = connected()

        const peerDoc = new Y.Doc()
        const peer = new Awareness(peerDoc)
        peer.setLocalState({ user: { id: 'ghost' } })
        ws.deliver(MSG_AWARENESS_UPDATE, encodeAwarenessUpdate(peer, [peer.clientID]))

        // An older broker names the departing BROKER id, which maps to no
        // awareness slot. The slot survives here by design — y-protocols'
        // own reaper is what eventually clears it.
        expect(() => ws.deliver(MSG_AWARENESS_UPDATE, new Uint8Array(0))).not.toThrow()
        expect(awareness.getStates().has(peer.clientID)).toBe(true)

        client.destroy()
        peer.destroy()
        peerDoc.destroy()
    })

    it('sends the local removal BEFORE closing the socket', () => {
        const { awareness, ws, client } = connected()
        awareness.setLocalState({ user: { id: 'me' } })
        ws.sent.length = 0
        ws.log.length = 0

        // What useRealtimeRoom's teardown does, in order.
        awareness.setLocalState(null)
        client.destroy()

        // A removal that never reaches the wire is the whole bug this
        // guards: pin the ordering so a refactor can't quietly invert it.
        expect(ws.log.indexOf('send')).toBeGreaterThanOrEqual(0)
        expect(ws.log.indexOf('send')).toBeLessThan(ws.log.indexOf('close'))

        // And the frame must actually clear the slot on a peer.
        const removal = ws.sent.find(f => frameType(f) === MSG_AWARENESS_UPDATE)
        expect(removal).toBeDefined()

        const observerDoc = new Y.Doc()
        const observer = new Awareness(observerDoc)
        observer.setLocalState(null)
        // Give the observer the departing client's slot first.
        applyAwarenessUpdate(observer, framePayload(removal as Uint8Array), 'test')
        expect(observer.getStates().has(awareness.clientID)).toBe(false)

        observer.destroy()
        observerDoc.destroy()
    })
})
