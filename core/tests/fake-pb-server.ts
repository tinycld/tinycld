import { pb } from '@tinycld/core/lib/pocketbase'
import { setResolvedAddress } from '@tinycld/core/lib/server-address'
import { type EventSourceListener, setRealtimeEventSource } from 'pbtsdb'
import { vi } from 'vitest'

const FAKE_SERVER = 'http://pb.test'

/** Answers one POST path: the parsed JSON body in, the JSON answer out. */
export type FakeRoute = (body: unknown, signal?: AbortSignal) => Promise<unknown>

const json = (value: unknown, status = 200) =>
    new Response(JSON.stringify(value), {
        status,
        headers: { 'content-type': 'application/json' },
    })

const aborted = () => new DOMException('The operation was aborted.', 'AbortError')

/** Rejects when `signal` aborts, as the browser's fetch does. */
function abortable<T>(answer: Promise<T>, signal?: AbortSignal | null): Promise<T> {
    if (!signal) return answer
    if (signal.aborted) return Promise.reject(aborted())
    return new Promise<T>((resolve, reject) => {
        signal.addEventListener('abort', () => reject(aborted()), { once: true })
        answer.then(resolve, reject)
    })
}

/**
 * A fake PocketBase server for a unit test of code that reads through the real `pb`
 * client and pbtsdb's realtime connection. It works at the network edge only: it
 * stubs the global `fetch` and gives pbtsdb a fake SSE stream through its public
 * `setRealtimeEventSource`, so no module of ours is mocked. A POST to a path in
 * `routes` gets the route's answer (a request the route has not answered rejects
 * when its signal aborts); the realtime subscriptions POST is accepted and recorded;
 * any other request is a 404. `emit(topic)` sends a message on a subscribed topic.
 *
 * Vitest gives each test file its own modules, so the server address and the
 * realtime source stay fake for the rest of the file; `vi.unstubAllGlobals()` puts
 * `fetch` back.
 */
export function fakePbServer(routes: Record<string, FakeRoute>) {
    const listeners = new Map<string, Set<EventSourceListener>>()
    let topics: string[] = []
    let connections = 0

    const fetchStub = async (input: RequestInfo | URL, init?: RequestInit) => {
        const path = new URL(String(input)).pathname
        const body = typeof init?.body === 'string' ? JSON.parse(init.body) : undefined
        if (path === '/api/realtime') {
            topics = (body as { subscriptions: string[] }).subscriptions
            return new Response(null, { status: 204 })
        }
        const route = routes[path]
        if (!route) return json({ message: `no route for ${path}` }, 404)
        return json(await abortable(route(body, init?.signal ?? undefined), init?.signal))
    }
    vi.stubGlobal('fetch', vi.fn(fetchStub))
    // The app resolves its server before `pb` sends anything; the test has none.
    setResolvedAddress(FAKE_SERVER)

    setRealtimeEventSource(pb, () => {
        connections += 1
        const clientId = `fake-client-${connections}`
        listeners.clear()
        const source = {
            addEventListener(type: string, listener: EventSourceListener) {
                const set = listeners.get(type) ?? new Set()
                set.add(listener)
                listeners.set(type, set)
            },
            removeEventListener(type: string, listener: EventSourceListener) {
                listeners.get(type)?.delete(listener)
            },
            close() {
                listeners.clear()
            },
        }
        // The server says hello once the stream is open, after the client listens.
        setTimeout(() => {
            for (const listener of listeners.get('PB_CONNECT') ?? []) {
                listener({ data: JSON.stringify({ clientId }), lastEventId: clientId })
            }
        }, 0)
        return source
    })

    return {
        /** The topics of the last subscriptions POST. */
        topics: () => topics,
        emit(topic: string) {
            for (const listener of listeners.get(topic) ?? []) listener({ data: '{}' })
        },
    }
}
