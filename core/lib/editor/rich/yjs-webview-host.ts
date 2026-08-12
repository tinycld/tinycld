import * as Y from 'yjs'
import { type EditorMessage, makeMessage } from '../message-bus/types'
import {
    decodeUpdate,
    encodeUpdate,
    YJS_UPDATE,
    type YjsUpdatePayload,
} from './webview/source/protocol'

/**
 * Host side of the Yjs relay: bridges the room's Y.Doc to the editor running
 * inside the WebView.
 *
 * The native side already holds an open room socket (cards' `useBoardPresence`
 * owns it, text's `useTextRoom` owns its own), so the WebView must NOT open its
 * own: a second connection both ships a credential into the page and makes the
 * local user appear twice in presence. Here the page is a pure participant: it
 * gets bytes, it sends bytes, and the host's existing socket does all the
 * networking. Carets ride the same arrangement — see `awareness-webview-host`.
 *
 * Both directions are guarded by RELAY_ORIGIN, which is what keeps a relay from
 * becoming an echo chamber:
 *
 *   - Updates applied to the host doc under RELAY_ORIGIN are not sent BACK to
 *     the WebView (it authored them).
 *   - RELAY_ORIGIN is deliberately distinct from the realtime client's
 *     REMOTE_ORIGIN/SYNC_ORIGIN, which are the only origins that client
 *     suppresses. So an edit typed in the WebView still reaches the server,
 *     while an edit that arrived FROM the server is not bounced back to it.
 */

/** Tags host-doc transactions whose updates came from the WebView. */
export const RELAY_ORIGIN: unique symbol = Symbol('editor:yjs-relay')

type PostMessage = (message: EditorMessage) => boolean

export interface YjsWebViewHostOptions {
    /** The room's Y.Doc. Never construct one for this — pass the live one. */
    doc: Y.Doc
    postMessage: PostMessage
}

export class YjsWebViewHost {
    private readonly doc: Y.Doc
    private readonly postMessage: PostMessage
    private readonly onDocUpdate: (update: Uint8Array, origin: unknown) => void
    private destroyed = false

    constructor(options: YjsWebViewHostOptions) {
        this.doc = options.doc
        this.postMessage = options.postMessage

        this.onDocUpdate = (update, origin) => {
            if (this.destroyed) return
            // The WebView authored this one; sending it back would loop.
            if (origin === RELAY_ORIGIN) return
            this.postMessage(makeMessage('yjs', YJS_UPDATE, { update: encodeUpdate(update) }))
        }
        this.doc.on('update', this.onDocUpdate)
    }

    /**
     * The host doc's full state, for the page to start from.
     *
     * Encoded as a Yjs update rather than sent as text: the page applies it to
     * its own doc, which is what makes the two documents the same document
     * rather than two copies that happen to start alike.
     */
    encodeState(): string {
        return encodeUpdate(Y.encodeStateAsUpdate(this.doc))
    }

    /** The host doc's clientID, for correlation only — the page cannot adopt it. */
    clientID(): number {
        return this.doc.clientID
    }

    /**
     * Feed a WebView message in. Returns true if this host consumed it, so
     * callers can stop routing it further.
     */
    handleMessage(message: EditorMessage): boolean {
        if (message.namespace !== 'yjs') return false
        if (message.type !== YJS_UPDATE) return true
        if (this.destroyed) return true
        const encoded = (message.payload as YjsUpdatePayload | undefined)?.update
        if (typeof encoded !== 'string' || encoded.length === 0) return true
        try {
            // RELAY_ORIGIN both stops the echo above and — because it is not
            // one of the realtime client's suppressed origins — lets this
            // update flow on to the server like any local edit.
            Y.applyUpdate(this.doc, decodeUpdate(encoded), RELAY_ORIGIN)
        } catch {
            // A malformed update must not take the editor down. Yjs is
            // convergent, so the next update from this peer carries the same
            // state and repairs the gap.
        }
        return true
    }

    destroy(): void {
        this.destroyed = true
        this.doc.off('update', this.onDocUpdate)
    }
}
