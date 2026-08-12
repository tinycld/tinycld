import { describe, expect, it } from 'vitest'
import * as Y from 'yjs'
import type { EditorMessage } from '../../message-bus/types'
import { makeMessage } from '../../message-bus/types'
import {
    decodeUpdate,
    encodeUpdate,
    YJS_UPDATE,
    type YjsUpdatePayload,
} from '../webview/source/protocol'
import { RELAY_ORIGIN, YjsWebViewHost } from '../yjs-webview-host'

// The relay is the whole feature on native: the host owns the room socket and
// the WebView owns the editor, so every keystroke crosses this bridge as
// base64 over a JSON pipe. These tests drive BOTH sides through the real
// encode/decode helpers — a stub that passed Uint8Arrays straight through
// would not exercise the part most likely to break.

const FIELD = 'card:abc'

/** Stands in for the page: its own Y.Doc, wired to the host by hand. */
function makePage() {
    const doc = new Y.Doc()
    const FROM_HOST = Symbol('from-host')
    const sent: EditorMessage[] = []
    doc.on('update', (update: Uint8Array, origin: unknown) => {
        if (origin === FROM_HOST) return
        sent.push(makeMessage('yjs', YJS_UPDATE, { update: encodeUpdate(update) }))
    })
    return {
        doc,
        sent,
        /** Apply a host→page message the way the page's relay hook does. */
        receive(message: EditorMessage) {
            const encoded = (message.payload as YjsUpdatePayload).update
            Y.applyUpdate(doc, decodeUpdate(encoded), FROM_HOST)
        },
        text: () => doc.getXmlFragment(FIELD).toString(),
    }
}

function makeHost(doc: Y.Doc) {
    const sent: EditorMessage[] = []
    const host = new YjsWebViewHost({
        doc,
        postMessage: message => {
            sent.push(message)
            return true
        },
    })
    return { host, sent }
}

describe('YjsWebViewHost', () => {
    it('seeds the page from the host doc, then converges in both directions', () => {
        const hostDoc = new Y.Doc()
        hostDoc.getXmlFragment(FIELD).insert(0, [new Y.XmlText('hello ')])
        const { host, sent } = makeHost(hostDoc)

        // Seed: the page starts from the host's state rather than from text,
        // which is what makes the two docs one document.
        const page = makePage()
        page.receive(makeMessage('yjs', YJS_UPDATE, { update: host.encodeState() }))
        expect(page.text()).toContain('hello')

        // Page → host.
        page.doc.getXmlFragment(FIELD).insert(1, [new Y.XmlText('world')])
        for (const message of page.sent) host.handleMessage(message)
        expect(hostDoc.getXmlFragment(FIELD).toString()).toContain('world')

        // Host → page (e.g. an edit that arrived on the room socket).
        sent.length = 0
        hostDoc.getXmlFragment(FIELD).insert(2, [new Y.XmlText('!')])
        for (const message of sent) page.receive(message)

        expect(page.text()).toBe(hostDoc.getXmlFragment(FIELD).toString())
        expect(page.text()).toContain('!')
    })

    it('does not echo an update it just applied from the page', () => {
        const hostDoc = new Y.Doc()
        const { host, sent } = makeHost(hostDoc)
        const page = makePage()

        page.doc.getXmlFragment(FIELD).insert(0, [new Y.XmlText('typed')])
        expect(page.sent).toHaveLength(1)

        sent.length = 0
        host.handleMessage(page.sent[0])

        // Posting this back would hand the page its own keystroke, which it
        // would apply and re-post: one keystroke, relayed forever.
        expect(sent).toHaveLength(0)
        expect(hostDoc.getXmlFragment(FIELD).toString()).toContain('typed')
    })

    it('relays a host update under an origin the realtime client forwards', () => {
        // RELAY_ORIGIN must NOT be one of the origins RealtimeClient
        // suppresses (REMOTE_ORIGIN / SYNC_ORIGIN), or an edit typed on the
        // phone would sync to the WebView's host doc and never reach anyone
        // else in the room.
        const hostDoc = new Y.Doc()
        const { host } = makeHost(hostDoc)
        const seen: unknown[] = []
        hostDoc.on('update', (_u: Uint8Array, origin: unknown) => seen.push(origin))

        const page = makePage()
        page.doc.getXmlFragment(FIELD).insert(0, [new Y.XmlText('x')])
        host.handleMessage(page.sent[0])

        expect(seen).toEqual([RELAY_ORIGIN])
    })

    it('survives a malformed update instead of taking the editor down', () => {
        const hostDoc = new Y.Doc()
        const { host } = makeHost(hostDoc)

        expect(() =>
            host.handleMessage(makeMessage('yjs', YJS_UPDATE, { update: 'not-a-real-update' }))
        ).not.toThrow()
        // Consumed either way, so nothing downstream tries to parse it as a
        // format action.
        expect(host.handleMessage(makeMessage('yjs', YJS_UPDATE, { update: '' }))).toBe(true)
    })

    it('ignores messages from other namespaces', () => {
        const hostDoc = new Y.Doc()
        const { host } = makeHost(hostDoc)
        expect(host.handleMessage(makeMessage('markdown', 'result', { markdown: 'x' }))).toBe(false)
        expect(host.handleMessage(makeMessage('app', 'escape', null))).toBe(false)
    })

    it('stops relaying once destroyed', () => {
        const hostDoc = new Y.Doc()
        const { host, sent } = makeHost(hostDoc)
        host.destroy()

        sent.length = 0
        hostDoc.getXmlFragment(FIELD).insert(0, [new Y.XmlText('after destroy')])
        expect(sent).toHaveLength(0)
    })

    it('reports the host clientID for correlation', () => {
        const hostDoc = new Y.Doc()
        const { host } = makeHost(hostDoc)
        expect(host.clientID()).toBe(hostDoc.clientID)
    })

    it('converges three peers, so the phone is a real participant', () => {
        // The shape that matters in production: a web peer on the room socket,
        // the phone's host doc, and the editor inside its WebView.
        const webPeer = new Y.Doc()
        const hostDoc = new Y.Doc()
        const { host, sent } = makeHost(hostDoc)
        const page = makePage()
        page.receive(makeMessage('yjs', YJS_UPDATE, { update: host.encodeState() }))

        // Web peer types; the room delivers it to the phone's doc.
        webPeer.getXmlFragment(FIELD).insert(0, [new Y.XmlText('from web ')])
        sent.length = 0
        Y.applyUpdate(hostDoc, Y.encodeStateAsUpdate(webPeer), 'remote')
        for (const message of sent) page.receive(message)
        expect(page.text()).toContain('from web')

        // Phone types in the WebView; it must reach the web peer.
        page.doc.getXmlFragment(FIELD).insert(1, [new Y.XmlText('from phone')])
        for (const message of page.sent) host.handleMessage(message)
        Y.applyUpdate(webPeer, Y.encodeStateAsUpdate(hostDoc), 'remote')

        const final = webPeer.getXmlFragment(FIELD).toString()
        expect(final).toContain('from web')
        expect(final).toContain('from phone')
        expect(hostDoc.getXmlFragment(FIELD).toString()).toBe(final)
    })
})
