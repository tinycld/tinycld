import type { BrowserContext, Page, Request } from '@playwright/test'

// The PocketBase fork keeps the realtime client of a closed connection
// detached for a grace period (minutes), so that a page that lost its network
// can resume it, and every write rule-checks the subscriptions of each
// detached client. The app releases its client on `pagehide`, but a Playwright
// context closes without `pagehide`, so every test would leave its clients
// detached and slow each later write of the run. Most specs get their page
// from Playwright's built-in fixtures, which close it after the test with no
// shared code in between, so the release is sent from Node after the close.
// Node and the browser both reach the server at the config's origin, so the
// release comes from the IP the fork binds the client to.

interface RealtimeClient {
    origin: string
    clientId: string
    token: string
}

const trackedContexts = new WeakSet<BrowserContext>()
const pendingReleases = new Set<Promise<void>>()

// A release must not hold up the next test when the server is gone.
const RELEASE_TIMEOUT_MS = 5_000

// pbtsdb's subscriptions POST: `{ clientId, subscriptions }` with the token the
// client subscribed with. A guest sends no token, and the fork never keeps a
// guest's client, so there is nothing to release for one.
function subscribedClient(request: Request): RealtimeClient | null {
    if (request.method() !== 'POST') return null
    const url = new URL(request.url())
    if (url.pathname !== '/api/realtime') return null
    const token = request.headers().authorization
    if (!token) return null
    const body = parsedBody(request)
    const clientId = body?.clientId
    if (typeof clientId !== 'string' || clientId === '') return null
    return { origin: url.origin, clientId, token }
}

function parsedBody(request: Request): { clientId?: unknown } | null {
    try {
        return request.postDataJSON() as { clientId?: unknown } | null
    } catch {
        return null
    }
}

// A service worker's request has no frame, and `frame()` throws for it.
function pageOf(request: Request): Page | null {
    if (request.serviceWorker()) return null
    try {
        return request.frame().page()
    } catch {
        return null
    }
}

// The app's own release request (core/lib/realtime-event-source.ts): the fork
// releases a client only for its own auth record, so it carries the token
// the client subscribed with.
function release(client: RealtimeClient) {
    const sent: Promise<void> = fetch(`${client.origin}/api/realtime/release`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Authorization: client.token },
        body: JSON.stringify({ clientId: client.clientId }),
        signal: AbortSignal.timeout(RELEASE_TIMEOUT_MS),
    })
        .then(response => {
            if (!response.ok) throw new Error(`release answered ${response.status}`)
        })
        .catch(error => {
            // A failed release only leaves the client to wait out the grace;
            // it must never fail the test that happens to be running.
            console.warn(`realtime release of client ${client.clientId} failed`, error)
        })
        .finally(() => {
            pendingReleases.delete(sent)
        })
    pendingReleases.add(sent)
}

/**
 * Releases the realtime client of each page of `context` when the page closes,
 * which a context close also does. Safe to call more than once per context.
 */
export function trackRealtimeClients(context: BrowserContext) {
    if (trackedContexts.has(context)) return
    trackedContexts.add(context)
    const clients = new Map<Page, RealtimeClient>()

    function releaseOnClose(page: Page) {
        page.once('close', () => {
            const client = clients.get(page)
            clients.delete(page)
            if (client) release(client)
        })
    }

    context.on('request', request => {
        const client = subscribedClient(request)
        if (!client) return
        const page = pageOf(request)
        if (!page) return
        const previous = clients.get(page)
        if (!previous) releaseOnClose(page)
        // A page that subscribes with a new client id has left its old client.
        if (previous && previous.clientId !== client.clientId) release(previous)
        clients.set(page, client)
    })
}

/** Waits until every release sent so far has an answer (or has failed). */
export async function settleRealtimeReleases(): Promise<void> {
    while (pendingReleases.size > 0) {
        await Promise.all(pendingReleases)
    }
}
