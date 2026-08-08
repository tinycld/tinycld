import { type EditorMessage, makeMessage } from '../message-bus/types'
import {
    MARKDOWN_GET,
    MARKDOWN_RESULT,
    MARKDOWN_SET,
    type MarkdownResultPayload,
} from './webview/source/protocol'

/**
 * Host side of the markdown channel: correlates `get` requests with the
 * WebView's `result` responses.
 *
 * TenTap ships `asyncMessages` for this shape, but it is not usable here — its
 * promise has no timeout and no reject path, so a WebView that dies mid-request
 * leaks a pending promise forever. `getMarkdown` sits on the save path, so that
 * failure mode would hang a save.
 *
 * The timeout therefore RESOLVES with the last known markdown rather than
 * rejecting. A save path that throws loses the user's text; returning the last
 * value we saw degrades to a slightly stale save, which is recoverable.
 */

/** Long enough to cover a slow first paint, short enough not to stall a save. */
const DEFAULT_TIMEOUT_MS = 3000

type PostMessage = (message: EditorMessage) => boolean

export interface MarkdownWebViewHostOptions {
    postMessage: PostMessage
    timeoutMs?: number
}

interface Pending {
    resolve: (markdown: string) => void
    timer: ReturnType<typeof setTimeout>
}

export class MarkdownWebViewHost {
    private readonly postMessage: PostMessage
    private readonly timeoutMs: number
    private readonly pending = new Map<string, Pending>()
    private nextId = 0
    /**
     * Last markdown the WebView reported. Seeded from the initial content so a
     * timeout before the first successful round-trip still returns the
     * document the user opened rather than an empty string.
     */
    private lastKnown = ''
    private destroyed = false

    constructor(options: MarkdownWebViewHostOptions) {
        this.postMessage = options.postMessage
        this.timeoutMs = options.timeoutMs ?? DEFAULT_TIMEOUT_MS
    }

    /** Seed the fallback value, e.g. with the editor's initial content. */
    seed(markdown: string): void {
        this.lastKnown = markdown
    }

    /**
     * Ask the WebView for the current document.
     *
     * Resolves with the last known value — never rejects — if the WebView is
     * not mounted, does not answer in time, or the host is torn down first.
     */
    get(): Promise<string> {
        if (this.destroyed) return Promise.resolve(this.lastKnown)
        const requestId = `md-${this.nextId++}`
        return new Promise<string>(resolve => {
            const settle = (markdown: string) => {
                const entry = this.pending.get(requestId)
                if (entry) {
                    clearTimeout(entry.timer)
                    this.pending.delete(requestId)
                }
                resolve(markdown)
            }
            const timer = setTimeout(() => settle(this.lastKnown), this.timeoutMs)
            this.pending.set(requestId, { resolve, timer })

            const sent = this.postMessage(makeMessage('markdown', MARKDOWN_GET, null, requestId))
            // The WebView isn't mounted yet — nothing will ever answer, so
            // don't make the caller wait out the timeout.
            if (!sent) settle(this.lastKnown)
        })
    }

    /** Replace the WebView's document. */
    set(markdown: string): void {
        if (this.destroyed) return
        this.lastKnown = markdown
        this.postMessage(makeMessage('markdown', MARKDOWN_SET, { markdown }))
    }

    /**
     * Feed a WebView message in. Returns true if it was a markdown message this
     * host consumed, so callers can stop routing it further.
     */
    handleMessage(message: EditorMessage): boolean {
        if (message.namespace !== 'markdown') return false
        if (message.type !== MARKDOWN_RESULT) return false
        const markdown = (message.payload as MarkdownResultPayload | undefined)?.markdown
        if (typeof markdown !== 'string') return true
        this.lastKnown = markdown
        // A response with no requestId is unsolicited — record the value but
        // don't try to settle a request that doesn't exist.
        if (message.requestId === undefined) return true
        const entry = this.pending.get(message.requestId)
        if (!entry) return true
        clearTimeout(entry.timer)
        this.pending.delete(message.requestId)
        entry.resolve(markdown)
        return true
    }

    /**
     * Drain every in-flight request on unmount so no caller is left awaiting a
     * promise the WebView can no longer answer.
     */
    destroy(): void {
        this.destroyed = true
        for (const [, entry] of this.pending) {
            clearTimeout(entry.timer)
            entry.resolve(this.lastKnown)
        }
        this.pending.clear()
    }
}
