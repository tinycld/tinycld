import type { Fetch } from '@tinycld/core/lib/read-only-retry'

// Platform-neutral pieces of the native multipart uploader
// (native-upload.native.ts). The native global fetch (expo/fetch since SDK 56)
// cannot stream a request body: its JS side turns every body — FormData parts
// included — into one Uint8Array before handing it to native code
// (`normalizeBodyInitAsync`, then `NativeRequest.start(..., requestBody:
// Uint8Array)`; Swift takes `Data`, Kotlin a `ByteArray`). So on native every
// multipart upload of a file on disk goes through expo-file-system's upload
// task instead, which streams from disk and reports progress.

export type FormDataEntries = [string, unknown][]

export type UploadMethod = 'POST' | 'PUT' | 'PATCH'

export type UploadProgress = (loaded: number, total: number) => void

/** A multipart upload expo-file-system's MULTIPART mode can send: one file plus string fields. */
export interface SingleFileUpload<F> {
    file: F
    fieldName: string
    parameters: Record<string, string>
}

export interface MultipartUploadRequest {
    url: string
    method: UploadMethod
    /** Request headers without Content-Type, which the uploader sets with its boundary. */
    headers: Record<string, string>
    entries: FormDataEntries
    onProgress?: UploadProgress
    signal?: AbortSignal
}

export interface MultipartUploader {
    /** Whether a FormData value is a file on disk the uploader streams. */
    isFile: (value: unknown) => boolean
    send: (request: MultipartUploadRequest) => Promise<Response>
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
 * The single-file form of a multipart body, or null when the body has a shape
 * expo-file-system's MULTIPART mode cannot express: it takes exactly one file,
 * and its string fields are a map, so a repeated field name would be lost.
 */
export function planSingleFileUpload<F>(
    entries: FormDataEntries,
    isFile: (value: unknown) => value is F
): SingleFileUpload<F> | null {
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

export interface MultipartSources<F> {
    isFile: (value: unknown) => value is F
    fileName: (file: F) => string
    fileType: (file: F) => string
    /** Writes the file's bytes to the sink, a chunk at a time. */
    copyFile: (file: F, write: (bytes: Uint8Array) => void) => Promise<void>
    readBlob: (blob: Blob) => Promise<Uint8Array>
}

// The field name and filename escaping browsers apply (WHATWG "multipart/
// form-data encoding algorithm"), so a quote or line break in a user's
// filename cannot break out of the header.
function escapeHeaderValue(value: string): string {
    return value.replace(/\r/g, '%0D').replace(/\n/g, '%0A').replace(/"/g, '%22')
}

export function makeBoundary(): string {
    const random = Math.random().toString(16).slice(2) + Math.random().toString(16).slice(2)
    return `----tinycld-form-${random}`
}

/**
 * Writes a multipart/form-data body for `entries` to `write`, streaming each
 * file part through `copyFile` so no file is ever held whole in memory.
 */
export async function writeMultipart<F>(
    entries: FormDataEntries,
    boundary: string,
    write: (bytes: Uint8Array) => void,
    sources: MultipartSources<F>
): Promise<void> {
    const encoder = new TextEncoder()
    const head = (name: string, filename: string | null, type: string | null) => {
        let text = `--${boundary}\r\nContent-Disposition: form-data; name="${escapeHeaderValue(name)}"`
        if (filename !== null) text += `; filename="${escapeHeaderValue(filename)}"`
        text += '\r\n'
        if (type !== null) text += `Content-Type: ${type}\r\n`
        write(encoder.encode(`${text}\r\n`))
    }
    for (const [key, value] of entries) {
        if (sources.isFile(value)) {
            head(
                key,
                sources.fileName(value),
                sources.fileType(value) || 'application/octet-stream'
            )
            await sources.copyFile(value, write)
        } else if (value instanceof Blob) {
            const name = 'name' in value && typeof value.name === 'string' ? value.name : 'blob'
            head(key, name, value.type || 'application/octet-stream')
            write(await sources.readBlob(value))
        } else {
            head(key, null, null)
            write(encoder.encode(String(value)))
        }
        write(encoder.encode('\r\n'))
    }
    write(encoder.encode(`--${boundary}--\r\n`))
}

function uploadMethod(method: string | undefined): UploadMethod | null {
    const upper = (method ?? 'GET').toUpperCase()
    return upper === 'POST' || upper === 'PUT' || upper === 'PATCH' ? upper : null
}

/** The method as one the native uploader supports, or an error naming it. */
export function requireUploadMethod(method: string | undefined): UploadMethod {
    const supported = uploadMethod(method)
    if (!supported) throw new TypeError(`Uploads must use POST, PUT or PATCH, not ${method}`)
    return supported
}

function requestUrl(url: RequestInfo | URL): string | null {
    if (typeof url === 'string') return url
    return url instanceof URL ? url.href : null
}

export function headerRecord(init: HeadersInit | undefined): Record<string, string> {
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
 * Wraps a fetch so a multipart body carrying a file on disk goes to the native
 * uploader, which streams it. Every other request reaches `fetchImpl`
 * untouched.
 */
export function withStreamedUploads(
    fetchImpl: Fetch,
    uploader: MultipartUploader,
    entriesOf: (body: BodyInit | null | undefined) => FormDataEntries | null = bodyEntries
): Fetch {
    return async (url, config) => {
        const entries = entriesOf(config?.body)
        const method = uploadMethod(config?.method)
        const target = requestUrl(url)
        if (!entries?.some(([, value]) => uploader.isFile(value)) || !method || !target) {
            return fetchImpl(url, config)
        }
        return uploader.send({
            url: target,
            method,
            headers: headerRecord(config?.headers),
            entries,
            signal: config?.signal ?? undefined,
        })
    }
}
