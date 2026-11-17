import { type Browser, expect, type Page, type Request, test } from '@playwright/test'
import { login, signInAsCollaborator } from './helpers.ts'

// The PocketBase fork keeps the realtime client of a closed connection
// detached for a grace period, so that a page that lost its network can
// resume it. A Playwright context closes without `pagehide`, so the app never
// releases its client, and every detached client costs each later write a
// rule check. These tests prove that the shared e2e setup releases the client
// of each page or context that ends.

interface RealtimeClient {
    origin: string
    clientId: string
    token: string
}

function isSubscribe(request: Request): boolean {
    return request.method() === 'POST' && new URL(request.url()).pathname === '/api/realtime'
}

function clientOf(request: Request): RealtimeClient {
    const body = request.postDataJSON() as { clientId?: string }
    const token = request.headers().authorization
    if (!body.clientId || !token) {
        throw new Error(`subscriptions POST without a client id or a token: ${request.postData()}`)
    }
    return { origin: new URL(request.url()).origin, clientId: body.clientId, token }
}

// Started before the page signs in or opens, so the app's first subscriptions
// POST is not missed. It resolves once the app has subscribed: a client that
// never subscribed is not kept detached, so closing earlier proves nothing.
function nextSubscribe(page: Page): Promise<RealtimeClient> {
    return page.waitForRequest(isSubscribe).then(clientOf)
}

// Reports whether the server still keeps the client detached: a resume of it
// is answered with `"resumed":true` in PB_CONNECT. A released client is gone,
// so the resume falls back to a new client. The request is aborted at once;
// the new client never subscribes, so the server does not keep it. Never probe
// a client whose page is still open: a resume takes the client from the page.
async function probeResumed(client: RealtimeClient): Promise<boolean> {
    const controller = new AbortController()
    try {
        const response = await fetch(
            `${client.origin}/api/realtime?resume=${encodeURIComponent(client.clientId)}`,
            {
                headers: { Authorization: client.token, Accept: 'text/event-stream' },
                signal: controller.signal,
            }
        )
        if (!response.ok || !response.body) {
            throw new Error(`realtime connect answered ${response.status}`)
        }
        const reader = response.body.getReader()
        const decoder = new TextDecoder()
        let buffer = ''
        for (;;) {
            const { done, value } = await reader.read()
            if (done) throw new Error('realtime stream ended before PB_CONNECT')
            buffer += decoder.decode(value, { stream: true })
            const events = buffer.split(/\r?\n\r?\n/)
            buffer = events.pop() ?? ''
            for (const event of events) {
                const lines = event.split(/\r?\n/)
                const name = lines
                    .find(line => line.startsWith('event:'))
                    ?.slice(6)
                    .trim()
                if (name !== 'PB_CONNECT') continue
                const data = lines
                    .filter(line => line.startsWith('data:'))
                    .map(line => line.slice(5).trim())
                    .join('\n')
                return (JSON.parse(data) as { resumed?: boolean }).resumed === true
            }
        }
    } finally {
        controller.abort()
    }
}

async function expectReleased(client: RealtimeClient) {
    await expect
        .poll(() => probeResumed(client), {
            message: `realtime client ${client.clientId} is released`,
            timeout: 10_000,
        })
        .toBe(false)
}

// signInAsCollaborator makes its context itself, and it subscribes before the
// helper returns. Wrapping the worker's browser for the length of the call is
// the only way to listen to that context from its start; it is put back after.
async function collaboratorWithClient(page: Page) {
    const browser = page.context().browser()
    if (!browser) throw new Error('the test page has no browser')
    const original = browser.newContext
    let subscribed: Promise<RealtimeClient> | undefined
    const listening: Browser['newContext'] = async options => {
        const context = await original.call(browser, options)
        subscribed = context.waitForEvent('request', { predicate: isSubscribe }).then(clientOf)
        return context
    }
    browser.newContext = listening
    try {
        const collaborator = await signInAsCollaborator(page)
        if (!subscribed) throw new Error('signInAsCollaborator made no context')
        return { collaborator, client: await subscribed }
    } finally {
        browser.newContext = original
    }
}

test.describe('realtime-release: a context or page that ends releases its client', () => {
    test('a context the spec makes', async ({ browser }) => {
        const context = await browser.newContext()
        const q = await context.newPage()
        const subscribed = nextSubscribe(q)
        await login(q)
        const client = await subscribed
        await context.close()

        await expectReleased(client)
    })

    test('the collaborator of signInAsCollaborator', async ({ page }) => {
        const { collaborator, client } = await collaboratorWithClient(page)
        await collaborator.close()

        await expectReleased(client)
    })

    // A guard: green today by design, because Chromium fires `pagehide` on
    // `page.close()` and the app releases its own client then.
    test('a second page of the signed-in context', async ({ page }) => {
        await login(page)
        const second = await page.context().newPage()
        const subscribed = nextSubscribe(second)
        await second.goto('/')
        const client = await subscribed
        await second.close()

        await expectReleased(client)
    })

    // A context close skips `pagehide`, so only the context-level release
    // covers pages that are still open when their context ends.
    test('two open pages of a context the spec makes', async ({ browser }) => {
        const context = await browser.newContext()
        const first = await context.newPage()
        const firstSubscribed = nextSubscribe(first)
        await login(first)
        const firstClient = await firstSubscribed
        const second = await context.newPage()
        const secondSubscribed = nextSubscribe(second)
        await second.goto('/')
        const secondClient = await secondSubscribed
        expect(secondClient.clientId, 'each page has its own realtime client').not.toBe(
            firstClient.clientId
        )
        await context.close()

        // One poll over both clients, so a failure shows the state of each.
        await expect
            .poll(async () => [await probeResumed(firstClient), await probeResumed(secondClient)], {
                message: 'the realtime clients of both pages are released',
                timeout: 10_000,
            })
            .toEqual([false, false])
    })
})

// These two tests are an ordered pair: the second observes what happens after
// the first test ends, when Playwright closes the built-in page and context.
// The config runs one file's tests in order in one worker (no fullyParallel),
// so the second runs after the first's fixtures are torn down.
test.describe('realtime-release: the built-in page releases its client when its test ends', () => {
    let builtInClient: RealtimeClient | undefined

    test('the built-in page signs in', async ({ page }) => {
        const subscribed = nextSubscribe(page)
        await login(page)
        builtInClient = await subscribed
    })

    test('the client of the previous test is released', async () => {
        expect(builtInClient, 'the previous test recorded its realtime client').toBeDefined()
        if (!builtInClient) return
        await expectReleased(builtInClient)
    })
})
