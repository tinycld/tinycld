import type { EditorMessage, EditorMessageNamespace } from '../message-bus/types'
import { makeMessage } from '../message-bus/types'

/**
 * Host side of a document channel: correlates `get` requests with the
 * WebView's `result` responses, and pushes `set`s the other way.
 *
 * Two channels share this shape — 'markdown' (card descriptions) and 'html'
 * (mail) — differing only in the namespace and the payload they carry, so the
 * mechanics live here once and each channel is a thin subclass.
 *
 * TenTap ships `asyncMessages` for this shape, but it is not usable here — its
 * promise has no timeout and no reject path, so a WebView that dies mid-request
 * leaks a pending promise forever. Both channels sit on a save path (a
 * description save, a draft save on compose close), so that failure mode would
 * hang a save — or, in mail's case, leave the compose window unclosable.
 *
 * The timeout therefore RESOLVES with the last known value rather than
 * rejecting. A save path that throws loses the user's text; returning the last
 * value we saw degrades to a slightly stale save, which is recoverable.
 */

/** Long enough to cover a slow first paint, short enough not to stall a save. */
const DEFAULT_TIMEOUT_MS = 3000

type PostMessage = (message: EditorMessage) => boolean

export interface DocumentWebViewHostOptions {
    postMessage: PostMessage
    timeoutMs?: number
}

interface ChannelShape<T> {
    namespace: EditorMessageNamespace
    /** Message types on the namespace. */
    types: { set: string; get: string; result: string }
    /** Value reported before any round-trip or seed. */
    initial: T
    /** Decode a `result` payload; null when it is malformed. */
    decodeResult: (payload: unknown) => T | null
    /** Encode the payload a `set` carries. */
    encodeSet: (value: T) => unknown
}

interface Pending<T> {
    resolve: (value: T) => void
    timer: ReturnType<typeof setTimeout>
}

export class DocumentWebViewHost<T> {
    private readonly postMessage: PostMessage
    private readonly timeoutMs: number
    private readonly shape: ChannelShape<T>
    private readonly pending = new Map<string, Pending<T>>()
    private nextId = 0
    /**
     * Last value the WebView reported. Seeded from the initial content so a
     * timeout before the first successful round-trip still returns the
     * document the user opened rather than an empty one.
     */
    private lastKnown: T
    /** Which warm-editor generation `lastKnown` belongs to. See seedGeneration. */
    private seededGeneration: number | null = null
    private destroyed = false

    constructor(shape: ChannelShape<T>, options: DocumentWebViewHostOptions) {
        this.shape = shape
        this.lastKnown = shape.initial
        this.postMessage = options.postMessage
        this.timeoutMs = options.timeoutMs ?? DEFAULT_TIMEOUT_MS
    }

    /** Seed the fallback value, e.g. with the editor's initial content. */
    seed(value: T): void {
        this.lastKnown = value
    }

    /**
     * Seed the fallback for a warm editor that has just been handed to another
     * surface.
     *
     * Distinct from {@link seed} because a handover has to invalidate requests
     * the PREVIOUS surface left in flight. Those resolve from `lastKnown` on
     * timeout, so re-seeding alone would hand one surface's text to the other's
     * pending save. Repeat calls for the same generation are ignored, which is
     * what lets a caller drive this from an effect.
     */
    seedGeneration(generation: number, value: T): void {
        if (generation === this.seededGeneration) return
        this.seededGeneration = generation
        // Anything still pending belongs to the surface being displaced. Settle
        // it with the text it was opened with rather than letting it time out
        // against the incoming surface's seed.
        for (const [id, entry] of this.pending) {
            clearTimeout(entry.timer)
            entry.resolve(this.lastKnown)
            this.pending.delete(id)
        }
        this.lastKnown = value
    }

    /**
     * Ask the WebView for the current document.
     *
     * Resolves with the last known value — never rejects — if the WebView is
     * not mounted, does not answer in time, or the host is torn down first.
     */
    get(): Promise<T> {
        if (this.destroyed) return Promise.resolve(this.lastKnown)
        const requestId = `${this.shape.namespace}-${this.nextId++}`
        return new Promise<T>(resolve => {
            const settle = (value: T) => {
                const entry = this.pending.get(requestId)
                if (entry) {
                    clearTimeout(entry.timer)
                    this.pending.delete(requestId)
                }
                resolve(value)
            }
            const timer = setTimeout(() => settle(this.lastKnown), this.timeoutMs)
            this.pending.set(requestId, { resolve, timer })

            const sent = this.postMessage(
                makeMessage(this.shape.namespace, this.shape.types.get, null, requestId)
            )
            // The WebView isn't mounted yet — nothing will ever answer, so
            // don't make the caller wait out the timeout.
            if (!sent) settle(this.lastKnown)
        })
    }

    /** Replace the WebView's document. */
    set(value: T): void {
        if (this.destroyed) return
        this.lastKnown = value
        this.postMessage(
            makeMessage(this.shape.namespace, this.shape.types.set, this.shape.encodeSet(value))
        )
    }

    /**
     * Feed a WebView message in. Returns true if it was a message on this
     * channel that the host consumed, so callers can stop routing it further.
     */
    handleMessage(message: EditorMessage): boolean {
        if (message.namespace !== this.shape.namespace) return false
        if (message.type !== this.shape.types.result) return false
        const value = this.shape.decodeResult(message.payload)
        if (value === null) return true
        this.lastKnown = value
        // A response with no requestId is unsolicited — record the value but
        // don't try to settle a request that doesn't exist.
        if (message.requestId === undefined) return true
        const entry = this.pending.get(message.requestId)
        if (!entry) return true
        clearTimeout(entry.timer)
        this.pending.delete(message.requestId)
        entry.resolve(value)
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
