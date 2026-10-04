import { isRecord } from '@tinycld/core/lib/errors'

// A server that is briefly unable to serve requests, e.g. while one server
// process hands over to another, answers 503 with code read_only (the
// database is mid-handover) or retry_later (the request arrived before any
// handler was ready). Either way the request never reached a handler, so
// sending it again is safe; doing it here, under every REST call the SDK and
// pbtsdb make, keeps the pause invisible to users unless it outlasts the
// retries.

export type Fetch = (url: RequestInfo | URL, config?: RequestInit) => Promise<Response>

export const READ_ONLY_MAX_RETRIES = 3
const DEFAULT_RETRY_AFTER_MS = 2000
const MAX_RETRY_AFTER_MS = 10000

// On web with a cross-origin API, `Retry-After` is not a CORS-safelisted
// response header, so the browser hides it from `headers.get` and this
// always falls through to the default — which is why the default equals the
// server's actual value rather than some unrelated fallback.
export function retryAfterMs(header: string | null): number {
    const seconds = Number(header)
    if (!Number.isFinite(seconds) || seconds <= 0) return DEFAULT_RETRY_AFTER_MS
    return Math.min(seconds * 1000, MAX_RETRY_AFTER_MS)
}

// The retryable-body predicate itself, split from response parsing so the
// XHR upload path (which never gets a `Response`, only a status and an
// already-parsed body) and the fetch path below share one definition.
export function isRetryableBody(status: number, body: unknown): boolean {
    if (status !== 503 || !isRecord(body)) return false
    return body.code === 'read_only' || body.code === 'retry_later'
}

async function isRetryableResponse(res: Response): Promise<boolean> {
    if (res.status !== 503) return false
    try {
        const body: unknown = await res.clone().json()
        return isRetryableBody(res.status, body)
    } catch {
        return false
    }
}

// Shared by every retry loop in this module (the fetch wrapper below and the
// XHR upload path in upload-file.ts): an abort must end the wait early
// rather than block up to 10 s for a request the caller has already given
// up on.
export const abortableWait = (ms: number, signal?: AbortSignal) =>
    new Promise<void>(resolve => {
        if (signal?.aborted) {
            resolve()
            return
        }
        const timer = setTimeout(resolve, ms)
        signal?.addEventListener(
            'abort',
            () => {
                clearTimeout(timer)
                resolve()
            },
            { once: true }
        )
    })

export function withReadOnlyRetry(
    fetchImpl: Fetch,
    sleep: (ms: number, signal?: AbortSignal) => Promise<void> = abortableWait
): Fetch {
    return async (url, config) => {
        let res = await fetchImpl(url, config)
        for (let attempt = 0; attempt < READ_ONLY_MAX_RETRIES; attempt++) {
            if (config?.signal?.aborted || !(await isRetryableResponse(res))) return res
            await sleep(retryAfterMs(res.headers.get('Retry-After')), config?.signal ?? undefined)
            if (config?.signal?.aborted) return res
            res = await fetchImpl(url, config)
        }
        return res
    }
}
