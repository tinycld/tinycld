// Reads the server's _logs for the app-boot beacon (POST /api/app/boot →
// `app-boot: rendered` with q.bundleId), the proof the running bundle's JS
// EXECUTED and mounted. The app posts this from BundleSentinel after the real
// provider tree commits — readable in a Release build where console.log is not.
// Mirrors logs-poller.ts (fetchAppUpdateCurrentIds/extractCurrentIds) but for the
// boot message + q.bundleId field.

interface LogsResponse {
    items?: Array<{ data?: Record<string, unknown> }>
}

// Pull data["q.bundleId"] from every boot-beacon log item that carries one, in
// response order. Tolerates items missing `data` or the key entirely.
export function extractBootBeaconIds(response: LogsResponse): string[] {
    const ids: string[] = []
    for (const item of response.items ?? []) {
        const id = item.data?.['q.bundleId']
        if (typeof id === 'string' && id.length > 0) ids.push(id)
    }
    return ids
}

// Query GET /api/logs for recent app-boot beacons and return their bundle ids.
// Used as the fetchCurrentIds-style dependency for pollForBundleId against a live
// server (same superuser token flow as logs-poller).
export async function fetchBootBeaconIds(serverUrl: string, token: string): Promise<string[]> {
    const filter = encodeURIComponent("message='app-boot: rendered'")
    const res = await fetch(`${serverUrl}/api/logs?filter=${filter}&sort=-created&perPage=20`, {
        headers: { Authorization: token },
    })
    if (!res.ok) {
        throw new Error(`fetchBootBeaconIds failed: ${res.status}`)
    }
    return extractBootBeaconIds((await res.json()) as LogsResponse)
}
