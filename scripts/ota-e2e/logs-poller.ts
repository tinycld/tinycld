// Minimal view of the PB `GET /api/logs` list response: each item's slog attrs
// live under `data`, keyed literally `q.currentId` (the dot is part of the key).
interface LogsResponse {
    items?: Array<{ data?: Record<string, unknown> }>
}

// Pull data["q.currentId"] from every log item that carries one, in response
// order. Tolerates items missing `data` or the key entirely.
export function extractCurrentIds(response: LogsResponse): string[] {
    const ids: string[] = []
    for (const item of response.items ?? []) {
        const id = item.data?.['q.currentId']
        if (typeof id === 'string' && id.length > 0) ids.push(id)
    }
    return ids
}

interface PollOpts {
    fetchCurrentIds: () => Promise<string[]>
    target: string
    timeoutMs: number
    intervalMs: number
    sleep?: (ms: number) => Promise<void>
    onPoll?: (ids: string[]) => void
}

const realSleep = (ms: number) => new Promise<void>(r => setTimeout(r, ms))

// Poll fetchCurrentIds until `target` appears among the returned ids, resolving
// with it. Elapsed time is accounted by accumulating intervalMs per loop rather
// than wall-clock, so the timeout is deterministic under an injected `sleep`.
export function pollForBundleId(opts: PollOpts): Promise<string> {
    const { fetchCurrentIds, target, timeoutMs, intervalMs, sleep = realSleep, onPoll } = opts
    return new Promise<string>((resolve, reject) => {
        let lastSeen: string[] = []
        let elapsed = 0

        async function loop() {
            try {
                const ids = await fetchCurrentIds()
                lastSeen = ids
                onPoll?.(ids)
                if (ids.includes(target)) {
                    resolve(target)
                    return
                }
                elapsed += intervalMs
                if (elapsed >= timeoutMs) {
                    reject(
                        new Error(
                            `pollForBundleId timed out after ${timeoutMs}ms waiting for ` +
                                `currentId=${target}; last-seen ids=[${lastSeen.join(', ')}]`
                        )
                    )
                    return
                }
                await sleep(intervalMs)
                await loop()
            } catch (err) {
                reject(err)
            }
        }

        void loop()
    })
}

// Authenticate as superuser → bearer token. Mirrors the tests/install superuser
// helper: POST /api/collections/_superusers/auth-with-password { identity, password }.
export async function superuserToken(
    serverUrl: string,
    identity: string,
    password: string
): Promise<string> {
    const res = await fetch(`${serverUrl}/api/collections/_superusers/auth-with-password`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ identity, password }),
    })
    if (!res.ok) {
        throw new Error(`superuserToken failed: ${res.status} ${await res.text()}`)
    }
    const body = (await res.json()) as { token?: string }
    if (!body.token) throw new Error('superuserToken: response missing token')
    return body.token
}

// Query GET /api/logs for recent app-update requests and return their currentIds.
// Used as the fetchCurrentIds dependency for pollForBundleId against a live server.
export async function fetchAppUpdateCurrentIds(
    serverUrl: string,
    token: string
): Promise<string[]> {
    const filter = encodeURIComponent("message='app-update: request'")
    const res = await fetch(`${serverUrl}/api/logs?filter=${filter}&sort=-created&perPage=20`, {
        headers: { Authorization: token },
    })
    if (!res.ok) {
        throw new Error(`fetchAppUpdateCurrentIds failed: ${res.status}`)
    }
    const json = (await res.json()) as LogsResponse
    return extractCurrentIds(json)
}
