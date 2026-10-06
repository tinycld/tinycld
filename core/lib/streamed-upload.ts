import type { Fetch } from '@tinycld/core/lib/read-only-retry'

// The native global fetch (expo/fetch since SDK 56) cannot stream a request
// body: its JS side turns every body — FormData parts included — into one
// Uint8Array before handing it to native code (`normalizeBodyInitAsync`, then
// `NativeRequest.start(..., requestBody: Uint8Array)`; Swift takes `Data`,
// Kotlin a `ByteArray`). A multipart upload of a file on disk would therefore
// hold the whole file in JS memory. This module routes such an upload to a
// native uploader that streams the file instead; native-upload-body.native.ts
// supplies that uploader.

export type FormDataEntries = [string, unknown][]

export type UploadMethod = 'POST' | 'PUT' | 'PATCH'

/** A multipart upload a native uploader can stream: one file plus string fields. */
export interface StreamedUpload<F> {
    file: F
    fieldName: string
    parameters: Record<string, string>
}

export interface StreamedUploadRequest<F> {
    url: string
    method: UploadMethod
    /** Request headers without Content-Type, which the uploader sets with its boundary. */
    headers: Record<string, string>
    signal?: AbortSignal
    upload: StreamedUpload<F>
}

export interface StreamedUploader<F> {
    /** Whether a FormData value is a file the uploader can stream. */
    isFile: (value: unknown) => value is F
    send: (request: StreamedUploadRequest<F>) => Promise<Response>
}

// FormData's typings promise string | File values, but React Native's FormData
// hands back whatever was appended, so the entries are read as unknown.
export function formDataEntries(formData: {
    forEach: (callback: (value: unknown, key: string) => void) => void
}): FormDataEntries {
    const entries: FormDataEntries = []
    formData.forEach((value, key) => {
        entries.push([key, value])
    })
    return entries
}

export function bodyEntries(body: BodyInit | null | undefined): FormDataEntries | null {
    return body instanceof FormData ? formDataEntries(body) : null
}

/**
 * The streamable form of a multipart body, or null when the body has a shape
 * a native uploader cannot express: it takes exactly one file, and its string
 * fields are a map, so a repeated field name would be lost.
 */
export function planStreamedUpload<F>(
    entries: FormDataEntries,
    isFile: (value: unknown) => value is F
): StreamedUpload<F> | null {
    let file: { fieldName: string; value: F } | null = null
    const parameters: Record<string, string> = {}
    const seen = new Set<string>()
    for (const [key, value] of entries) {
        if (isFile(value)) {
            if (file) return null
            file = { fieldName: key, value }
        } else if (typeof value === 'string') {
            if (seen.has(key)) return null
            seen.add(key)
            parameters[key] = value
        } else {
            return null
        }
    }
    if (!file) return null
    return { file: file.value, fieldName: file.fieldName, parameters }
}

function uploadMethod(method: string | undefined): UploadMethod | null {
    const upper = (method ?? 'GET').toUpperCase()
    return upper === 'POST' || upper === 'PUT' || upper === 'PATCH' ? upper : null
}

function requestUrl(url: RequestInfo | URL): string | null {
    if (typeof url === 'string') return url
    return url instanceof URL ? url.href : null
}

function headerRecord(init: HeadersInit | undefined): Record<string, string> {
    const record: Record<string, string> = {}
    new Headers(init).forEach((value, key) => {
        if (key.toLowerCase() !== 'content-type') record[key] = value
    })
    return record
}

const NULL_BODY_STATUSES = new Set([101, 204, 205, 304])

/** Presents a native upload's result as the fetch `Response` its caller expects. */
export function uploadResultToResponse(
    result: { body: string; status: number; headers: Record<string, string> },
    url: string
): Response {
    const body = NULL_BODY_STATUSES.has(result.status) ? null : result.body
    const response = new Response(body, { status: result.status, headers: result.headers })
    // The PocketBase SDK reports `response.url` in its errors.
    Object.defineProperty(response, 'url', { value: url })
    return response
}

/**
 * Wraps a fetch so a one-file multipart upload goes to `uploader` and streams
 * from disk. Every other request reaches `fetchImpl` untouched — including a
 * multipart body the uploader cannot express (several files, a repeated
 * field), which the platform fetch then sends as it always would.
 */
export function withStreamedUploads<F>(
    fetchImpl: Fetch,
    uploader: StreamedUploader<F>,
    entriesOf: (body: BodyInit | null | undefined) => FormDataEntries | null = bodyEntries
): Fetch {
    return async (url, config) => {
        const entries = entriesOf(config?.body)
        const upload = entries ? planStreamedUpload(entries, uploader.isFile) : null
        const method = uploadMethod(config?.method)
        const target = requestUrl(url)
        if (!upload || !method || !target) return fetchImpl(url, config)
        return uploader.send({
            url: target,
            method,
            headers: headerRecord(config?.headers),
            signal: config?.signal ?? undefined,
            upload,
        })
    }
}
