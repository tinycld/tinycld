import { isRecord } from '@tinycld/core/lib/errors'

// A server briefly refuses writes while a new server process migrates the
// database it shares with the old one (503 with code read_only). The request
// never reached a handler, so sending the same request again is safe; doing
// it here, under every REST call the SDK and pbtsdb make, keeps the pause
// invisible to users unless it outlasts the retries.

export type Fetch = (url: RequestInfo | URL, config?: RequestInit) => Promise<Response>

export const READ_ONLY_MAX_RETRIES = 3
const DEFAULT_RETRY_AFTER_MS = 2000
const MAX_RETRY_AFTER_MS = 10000

export function retryAfterMs(header: string | null): number {
    const seconds = Number(header)
    if (!Number.isFinite(seconds) || seconds <= 0) return DEFAULT_RETRY_AFTER_MS
    return Math.min(seconds * 1000, MAX_RETRY_AFTER_MS)
}

async function isReadOnly(res: Response): Promise<boolean> {
    if (res.status !== 503) return false
    try {
        const body: unknown = await res.clone().json()
        return isRecord(body) && body.code === 'read_only'
    } catch {
        return false
    }
}

const wait = (ms: number) => new Promise<void>(resolve => setTimeout(resolve, ms))

export function withReadOnlyRetry(
    fetchImpl: Fetch,
    sleep: (ms: number) => Promise<void> = wait
): Fetch {
    return async (url, config) => {
        let res = await fetchImpl(url, config)
        for (let attempt = 0; attempt < READ_ONLY_MAX_RETRIES; attempt++) {
            if (config?.signal?.aborted || !(await isReadOnly(res))) return res
            await sleep(retryAfterMs(res.headers.get('Retry-After')))
            res = await fetchImpl(url, config)
        }
        return res
    }
}
