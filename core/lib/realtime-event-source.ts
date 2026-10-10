import type { EventSourceLike, EventSourceListener, EventSourceMessage } from 'pbtsdb'
import type PocketBase from 'pocketbase'
import { log } from './logger'

// pbtsdb's realtime connection, opened with `fetch` instead of `EventSource`.
//
// The browser `EventSource` cannot send headers, and the server resumes a
// dropped connection (replaying the events it missed instead of making every
// collection reload) only when the SSE request carries the same
// `Authorization` as the client's subscriptions. React Native has no
// `EventSource` at all, and the `react-native-sse` polyfill reconnects on its
// own when the server ends a stream, so pbtsdb never learns the stream ended
// and the server never sees a resume.
//
// pbtsdb learns that a connection ended only from an `error` event, and it
// reconnects itself. So this source dispatches `error` exactly once, on ANY
// end of the stream — a clean close by the server included — and never
// reconnects.

export interface SseMessage {
    type: string
    data: string
    lastEventId: string
}

interface StreamingResponse {
    ok: boolean
    status: number
    body: ReadableStream<Uint8Array> | null
}

/** A `fetch` whose response body streams, such as `serverFetch`. */
export type StreamingFetch = (
    url: string,
    init: { headers: Record<string, string>; signal: AbortSignal }
) => Promise<StreamingResponse>

/**
 * Parses an SSE byte stream, already decoded to text, to the WHATWG rules.
 * Chunks may split a line anywhere, including between the `\r` and `\n` of
 * a CRLF.
 */
export function createSseParser(onMessage: (message: SseMessage) => void) {
    let buffer = ''
    let data: string[] = []
    let eventType = ''
    let lastEventId = ''

    function dispatch() {
        if (data.length > 0) {
            onMessage({ type: eventType || 'message', data: data.join('\n'), lastEventId })
        }
        data = []
        eventType = ''
    }

    function processLine(line: string) {
        if (line === '') {
            dispatch()
            return
        }
        if (line.startsWith(':')) return

        const colon = line.indexOf(':')
        const field = colon === -1 ? line : line.slice(0, colon)
        let value = colon === -1 ? '' : line.slice(colon + 1)
        if (value.startsWith(' ')) value = value.slice(1)

        if (field === 'data') data.push(value)
        else if (field === 'event') eventType = value
        else if (field === 'id' && !value.includes('\0')) lastEventId = value
    }

    return {
        feed(chunk: string) {
            buffer += chunk
            let start = 0
            for (let i = 0; i < buffer.length; i++) {
                const char = buffer[i]
                if (char !== '\r' && char !== '\n') continue
                // a `\r` at the end of the chunk may be the first half of a CRLF
                if (char === '\r' && i === buffer.length - 1) break
                processLine(buffer.slice(start, i))
                if (char === '\r' && buffer[i + 1] === '\n') i++
                start = i + 1
            }
            buffer = buffer.slice(start)
        },
    }
}

interface FetchEventSourceInit {
    fetch: StreamingFetch
    headers: Record<string, string>
}

export class FetchEventSource implements EventSourceLike {
    private readonly listeners = new Map<string, Set<EventSourceListener>>()
    private readonly controller = new AbortController()
    private ended = false

    constructor(url: string, init: FetchEventSourceInit) {
        void this.run(url, init)
    }

    addEventListener(type: string, listener: EventSourceListener) {
        let set = this.listeners.get(type)
        if (!set) {
            set = new Set()
            this.listeners.set(type, set)
        }
        set.add(listener)
    }

    removeEventListener(type: string, listener: EventSourceListener) {
        this.listeners.get(type)?.delete(listener)
    }

    /** Ends the connection. Dispatches nothing: the caller asked for it. */
    close() {
        this.ended = true
        this.controller.abort()
    }

    private async run(url: string, init: FetchEventSourceInit) {
        // Called unbound: the browser's fetch throws "Illegal invocation" when
        // called as a method of another object, such as `init.fetch(...)`.
        const { fetch } = init
        try {
            const response = await fetch(url, {
                headers: { Accept: 'text/event-stream', ...init.headers },
                signal: this.controller.signal,
            })
            if (!response.ok || !response.body) {
                throw new Error(`realtime stream responded ${response.status}`)
            }

            const reader = response.body.getReader()
            const decoder = new TextDecoder()
            const parser = createSseParser(message => {
                if (!this.ended) this.dispatch(message.type, message)
            })
            for (;;) {
                const { done, value } = await reader.read()
                if (done || this.ended) break
                parser.feed(decoder.decode(value, { stream: true }))
            }
        } catch (error) {
            if (!this.ended) log.debug('core.realtime', 'realtime stream failed', { error })
        }

        if (this.ended) return
        this.ended = true
        this.controller.abort()
        this.dispatch('error', {})
    }

    private dispatch(type: string, event: EventSourceMessage) {
        for (const listener of [...(this.listeners.get(type) ?? [])]) listener(event)
    }
}

/**
 * The factory for `setRealtimeEventSource(pb, …)`. pbtsdb calls it on each
 * connect, so the token read here is the current one, refreshed included.
 * A guest sends no `Authorization`, and the server never resumes a guest.
 */
export function realtimeEventSourceFactory(pb: PocketBase, fetch: StreamingFetch) {
    return (url: string) => {
        const token = pb.authStore.token
        const headers: Record<string, string> = token ? { Authorization: token } : {}
        return new FetchEventSource(url, { fetch, headers })
    }
}
