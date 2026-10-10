import PocketBase from 'pocketbase'
import { describe, expect, it } from 'vitest'
import {
    createSseParser,
    FetchEventSource,
    realtimeEventSourceFactory,
    type SseMessage,
    type StreamingFetch,
} from '../realtime-event-source'

function parse(chunks: string[]): SseMessage[] {
    const messages: SseMessage[] = []
    const parser = createSseParser(message => messages.push(message))
    for (const chunk of chunks) parser.feed(chunk)
    return messages
}

// A server stream the test writes to and ends by hand.
function fakeServer(status = 200) {
    let controller: ReadableStreamDefaultController<Uint8Array> | undefined
    const body = new ReadableStream<Uint8Array>({
        start(c) {
            controller = c
        },
    })
    const requests: { url: string; headers: Record<string, string>; signal: AbortSignal }[] = []
    // The browser's fetch refuses to run as a method of another object
    // ("Illegal invocation"), so the fake refuses too.
    const fetch: StreamingFetch = async function (this: unknown, url, init) {
        if (this !== undefined) throw new TypeError('Illegal invocation')
        requests.push({ url, ...init })
        return { ok: status >= 200 && status < 300, status, body }
    }
    const encoder = new TextEncoder()
    return {
        fetch,
        requests,
        write: (text: string) => controller?.enqueue(encoder.encode(text)),
        writeBytes: (bytes: Uint8Array) => controller?.enqueue(bytes),
        end: () => controller?.close(),
    }
}

function collect(source: FetchEventSource, ...types: string[]) {
    const events: { type: string; data?: string; lastEventId?: string }[] = []
    for (const type of types) {
        source.addEventListener(type, event => {
            events.push({
                type,
                data: event.data === undefined ? undefined : String(event.data),
                lastEventId: event.lastEventId ?? undefined,
            })
        })
    }
    return events
}

async function settle() {
    for (let i = 0; i < 10; i++) await new Promise(resolve => setTimeout(resolve, 0))
}

describe('createSseParser', () => {
    it('parses the PocketBase event shape', () => {
        expect(parse(['id:client-1\nevent:PB_CONNECT\ndata:{"clientId":"client-1"}\n\n'])).toEqual([
            { type: 'PB_CONNECT', data: '{"clientId":"client-1"}', lastEventId: 'client-1' },
        ])
    })

    it('joins a line split across chunks', () => {
        expect(parse(['ev', 'ent:to', 'pic\nda', 'ta:{"a"', ':1}\n', '\n'])).toEqual([
            { type: 'topic', data: '{"a":1}', lastEventId: '' },
        ])
    })

    it('accepts CRLF, CR and LF line ends, and a CRLF split across chunks', () => {
        expect(parse(['data:a\r', '\n\r', '\ndata:b\r\rdata:c\n\n'])).toEqual([
            { type: 'message', data: 'a', lastEventId: '' },
            { type: 'message', data: 'b', lastEventId: '' },
            { type: 'message', data: 'c', lastEventId: '' },
        ])
    })

    it('joins multi-line data, strips one leading space and skips comments', () => {
        expect(parse([': keep-alive\ndata:  one\ndata:two\n\n'])).toEqual([
            { type: 'message', data: ' one\ntwo', lastEventId: '' },
        ])
    })

    it('keeps the last id for later events and dispatches nothing without data', () => {
        expect(parse(['id:x\nevent:ignored\n\ndata:1\n\nid\ndata:2\n\n'])).toEqual([
            { type: 'message', data: '1', lastEventId: 'x' },
            { type: 'message', data: '2', lastEventId: '' },
        ])
    })

    it('keeps an event that is not complete until its blank line arrives', () => {
        expect(parse(['data:partial\n'])).toEqual([])
    })
})

describe('FetchEventSource', () => {
    it('sends the headers and dispatches each event to its listeners', async () => {
        const server = fakeServer()
        const source = new FetchEventSource('http://pb.test/api/realtime', {
            fetch: server.fetch,
            headers: { Authorization: 'token-1' },
        })
        const events = collect(source, 'PB_CONNECT', 'posts')

        server.write('id:client-1\nevent:PB_CONNECT\ndata:{"clientId":"client-1"}\n\n')
        server.write('id:client-1\nevent:posts\ndata:{"seq":1}\n\n')
        await settle()

        expect(server.requests[0].url).toBe('http://pb.test/api/realtime')
        expect(server.requests[0].headers).toEqual({
            Accept: 'text/event-stream',
            Authorization: 'token-1',
        })
        expect(events).toEqual([
            { type: 'PB_CONNECT', data: '{"clientId":"client-1"}', lastEventId: 'client-1' },
            { type: 'posts', data: '{"seq":1}', lastEventId: 'client-1' },
        ])
    })

    it('decodes a character split across chunks', async () => {
        const server = fakeServer()
        const source = new FetchEventSource('http://pb.test', { fetch: server.fetch, headers: {} })
        const events = collect(source, 'message')

        const bytes = new TextEncoder().encode('data:é\n\n')
        server.writeBytes(bytes.slice(0, 6))
        server.writeBytes(bytes.slice(6))
        await settle()

        expect(events.map(e => e.data)).toEqual(['é'])
    })

    it('dispatches error once when the server ends the stream', async () => {
        const server = fakeServer()
        const source = new FetchEventSource('http://pb.test', { fetch: server.fetch, headers: {} })
        const events = collect(source, 'error')

        server.end()
        await settle()

        expect(events).toHaveLength(1)
        expect(server.requests[0].signal.aborted).toBe(true)
    })

    it('dispatches error on a response that is not 2xx', async () => {
        const server = fakeServer(503)
        const source = new FetchEventSource('http://pb.test', { fetch: server.fetch, headers: {} })
        const events = collect(source, 'error')

        await settle()

        expect(events).toHaveLength(1)
    })

    it('dispatches error when the request fails', async () => {
        const fetch: StreamingFetch = async () => {
            throw new TypeError('network down')
        }
        const source = new FetchEventSource('http://pb.test', { fetch, headers: {} })
        const events = collect(source, 'error')

        await settle()

        expect(events).toHaveLength(1)
    })

    it('dispatches nothing after close and aborts the request', async () => {
        const server = fakeServer()
        const source = new FetchEventSource('http://pb.test', { fetch: server.fetch, headers: {} })
        const events = collect(source, 'error', 'message')
        await settle()

        source.close()
        server.write('data:late\n\n')
        server.end()
        await settle()

        expect(server.requests[0].signal.aborted).toBe(true)
        expect(events).toEqual([])
    })

    it('stops calling a removed listener', async () => {
        const server = fakeServer()
        const source = new FetchEventSource('http://pb.test', { fetch: server.fetch, headers: {} })
        const removed: string[] = []
        const listener = () => removed.push('called')
        source.addEventListener('message', listener)
        source.removeEventListener('message', listener)
        const events = collect(source, 'message')

        server.write('data:x\n\n')
        await settle()

        expect(removed).toEqual([])
        expect(events.map(e => e.data)).toEqual(['x'])
    })
})

describe('realtimeEventSourceFactory', () => {
    it('reads the token on each connect and sends none for a guest', async () => {
        const pb = new PocketBase('http://pb.test')
        const server = fakeServer()
        const open = realtimeEventSourceFactory(pb, server.fetch)
        const record = { id: 'user00000000000', collectionId: 'users', collectionName: 'users' }

        open('http://pb.test/api/realtime').close()
        pb.authStore.save('token-1', record)
        open('http://pb.test/api/realtime').close()
        pb.authStore.save('token-2', record)
        open('http://pb.test/api/realtime').close()
        await settle()

        expect(server.requests.map(r => r.headers.Authorization)).toEqual([
            undefined,
            'token-1',
            'token-2',
        ])
    })
})
