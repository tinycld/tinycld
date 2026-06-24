// Reads the server's pkg_bad_bundle collection (the durable record of a device
// that crash-looped a bundle and rolled back) so the crash-rollback E2E can
// assert the rollback was REPORTED with a meaningful reason — not just that it
// happened. Mirrors logs-poller.ts: plain fetch + an injectable sleep for
// deterministic tests.

export interface BadBundleRow {
    bundleId: string
    reports: number
    lastError: string
}

interface BadBundleResponse {
    items?: Array<{ bundle_id?: string; reports?: number; last_error?: string }>
}

export function extractBadBundles(response: BadBundleResponse): BadBundleRow[] {
    return (response.items ?? []).map(it => ({
        bundleId: it.bundle_id ?? '',
        reports: it.reports ?? 0,
        lastError: it.last_error ?? '',
    }))
}

interface PollOpts {
    fetchRows: () => Promise<BadBundleRow[]>
    target: string
    timeoutMs: number
    intervalMs: number
    sleep?: (ms: number) => Promise<void>
    onPoll?: (rows: BadBundleRow[]) => void
}

const realSleep = (ms: number) => new Promise<void>(r => setTimeout(r, ms))

// Poll until the target bundle has a pkg_bad_bundle row with a NON-EMPTY
// last_error (the captured rollback reason / regex detail). A row with an empty
// last_error is the failure the manual repro hit — we treat it as "not yet
// captured" and keep waiting, then surface it in the timeout message.
export function pollForBadBundle(opts: PollOpts): Promise<BadBundleRow> {
    const { fetchRows, target, timeoutMs, intervalMs, sleep = realSleep, onPoll } = opts
    return new Promise<BadBundleRow>((resolve, reject) => {
        let lastSeen: BadBundleRow[] = []
        let elapsed = 0
        async function loop() {
            try {
                const rows = await fetchRows()
                lastSeen = rows
                onPoll?.(rows)
                const hit = rows.find(r => r.bundleId === target && r.lastError !== '')
                if (hit) {
                    resolve(hit)
                    return
                }
                elapsed += intervalMs
                if (elapsed >= timeoutMs) {
                    reject(
                        new Error(
                            `pollForBadBundle timed out after ${timeoutMs}ms waiting for a ` +
                                `pkg_bad_bundle row with bundle_id=${target} and a non-empty last_error; ` +
                                `last-seen=${JSON.stringify(lastSeen)}`
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

// Fetch pkg_bad_bundle rows via the superuser API (same token flow as logs-poller).
export async function fetchBadBundles(serverUrl: string, token: string): Promise<BadBundleRow[]> {
    const res = await fetch(
        `${serverUrl}/api/collections/pkg_bad_bundle/records?sort=-updated&perPage=50`,
        {
            headers: { Authorization: token },
        }
    )
    if (!res.ok) {
        throw new Error(`fetchBadBundles failed: ${res.status}`)
    }
    return extractBadBundles((await res.json()) as BadBundleResponse)
}

// Existence probe for a collection: PocketBase returns 200 for a known
// collection's records endpoint (even when empty) and 404 for an unknown one.
// Any other status (transient mid-restart) returns null so the caller retries.
// Mirrors tests/install/todo-install.spec.ts collectionExists.
export async function collectionExists(
    serverUrl: string,
    token: string,
    name: string,
    fetchFn: typeof fetch = fetch
): Promise<boolean | null> {
    let res: Response
    try {
        res = await fetchFn(`${serverUrl}/api/collections/${name}/records?perPage=1`, {
            headers: { Authorization: token },
        })
    } catch {
        return null
    }
    if (res.status === 200) return true
    if (res.status === 404) return false
    return null
}
