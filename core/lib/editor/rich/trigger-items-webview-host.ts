import { type EditorMessage, makeMessage } from '../message-bus/types'
import type { TriggerItem } from './triggers'
import { APP_TRIGGER_ITEMS, type TriggerItemsPayload } from './webview/source/protocol'

/**
 * Host side of the trigger-roster channel: pushes each trigger's candidate pool
 * into the WebView whenever it actually changes.
 *
 * The page cannot compute candidates itself — they come from a live database
 * query on the host, and the page is a prebuilt bundle a closure cannot cross.
 * So the host pushes and the page filters locally, which keeps typing off the
 * bridge entirely.
 *
 * "Actually changes" is the whole job. The roster arrives from a live query
 * that re-emits a fresh array on unrelated writes, so posting on every render
 * would flood the bridge with identical payloads. Comparing the serialized form
 * is the same identity-skip the find-replace channel uses, and it is cheap
 * against a roster of tens of members.
 */
type PostMessage = (message: EditorMessage) => boolean

export class TriggerItemsWebViewHost {
    private readonly postMessage: PostMessage
    /** Last payload sent per trigger id, serialized for comparison. */
    private readonly sent = new Map<string, string>()

    constructor(options: { postMessage: PostMessage }) {
        this.postMessage = options.postMessage
    }

    /**
     * Push a roster if it differs from the last one sent.
     *
     * Returns whether anything went over the wire, which is what makes the
     * skip observable in a test.
     */
    push(triggerId: string, items: TriggerItem[]): boolean {
        const serialized = JSON.stringify(items)
        if (this.sent.get(triggerId) === serialized) return false
        this.sent.set(triggerId, serialized)
        const payload: TriggerItemsPayload = { triggerId, items }
        return this.postMessage(makeMessage('app', APP_TRIGGER_ITEMS, payload))
    }

    /**
     * Forget what has been sent, so the next push re-sends.
     *
     * Called when the page reloads: the new page's store is empty, but the
     * host's memo of what it sent is not, so without this a roster unchanged
     * across the reload would never be re-pushed and the popover would offer
     * nothing.
     */
    reset(): void {
        this.sent.clear()
    }
}
