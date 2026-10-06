import type { Fetch } from '@tinycld/core/lib/read-only-retry'

// React Native's FormData accepts a file on disk as a `{ uri, name, type }`
// literal, and every native upload in the app builds its body that way. The
// native global `fetch` (expo/fetch since SDK 56) rejects such parts with
// "Unsupported FormDataPart implementation", so a body that carries one is
// encoded here into a finished multipart payload, which any fetch accepts.

export interface NativeFilePart {
    uri: string
    name?: string
    type?: string
}

export interface MultipartReaders {
    /** The bytes of the file a native part points at. */
    readFile: (uri: string) => Promise<Uint8Array>
    /** The bytes of a Blob part. */
    readBlob: (blob: Blob) => Promise<Uint8Array>
}

export type FormDataEntries = [string, unknown][]

export interface MultipartBody {
    body: Uint8Array<ArrayBuffer>
    contentType: string
}

export function isNativeFilePart(value: unknown): value is NativeFilePart {
    if (typeof value !== 'object' || value === null || value instanceof Blob) return false
    return 'uri' in value && typeof value.uri === 'string'
}

// FormData's typings promise string | File values, but React Native's FormData
// hands back whatever was appended, so the entries are read as unknown.
export interface FormDataLike {
    forEach: (callback: (value: unknown, key: string) => void) => void
}

export function formDataEntries(formData: FormDataLike): FormDataEntries {
    const entries: FormDataEntries = []
    formData.forEach((value, key) => {
        entries.push([key, value])
    })
    return entries
}

// The field name and filename escaping browsers apply (WHATWG "multipart/
// form-data encoding algorithm"), so a quote or line break in a user's
// filename cannot break out of the header.
function escapeHeaderValue(value: string): string {
    return value.replace(/\r/g, '%0D').replace(/\n/g, '%0A').replace(/"/g, '%22')
}

function fileNameFromUri(uri: string): string {
    const last = uri.split(/[/\\]/).pop() ?? ''
    return last.split('?')[0] || 'file'
}

function makeBoundary(): string {
    const random = Math.random().toString(16).slice(2) + Math.random().toString(16).slice(2)
    return `----tinycld-form-${random}`
}

async function partBytes(
    value: unknown,
    readers: MultipartReaders
): Promise<{
    filename: string | null
    type: string | null
    bytes: Uint8Array
}> {
    if (isNativeFilePart(value)) {
        return {
            filename: value.name || fileNameFromUri(value.uri),
            type: value.type || 'application/octet-stream',
            bytes: await readers.readFile(value.uri),
        }
    }
    if (value instanceof Blob) {
        const name = 'name' in value && typeof value.name === 'string' ? value.name : 'blob'
        return {
            filename: name,
            type: value.type || 'application/octet-stream',
            bytes: await readers.readBlob(value),
        }
    }
    return { filename: null, type: null, bytes: new TextEncoder().encode(String(value)) }
}

export async function encodeMultipart(
    entries: FormDataEntries,
    readers: MultipartReaders,
    boundary: string = makeBoundary()
): Promise<MultipartBody> {
    const encoder = new TextEncoder()
    const chunks: Uint8Array[] = []
    for (const [key, value] of entries) {
        const part = await partBytes(value, readers)
        let head = `--${boundary}\r\nContent-Disposition: form-data; name="${escapeHeaderValue(key)}"`
        if (part.filename !== null) head += `; filename="${escapeHeaderValue(part.filename)}"`
        head += '\r\n'
        if (part.type !== null) head += `Content-Type: ${part.type}\r\n`
        chunks.push(encoder.encode(`${head}\r\n`), part.bytes, encoder.encode('\r\n'))
    }
    chunks.push(encoder.encode(`--${boundary}--\r\n`))

    const body = new Uint8Array(chunks.reduce((total, chunk) => total + chunk.byteLength, 0))
    let offset = 0
    for (const chunk of chunks) {
        body.set(chunk, offset)
        offset += chunk.byteLength
    }
    return { body, contentType: `multipart/form-data; boundary=${boundary}` }
}

export function bodyEntries(body: BodyInit | null | undefined): FormDataEntries | null {
    return body instanceof FormData ? formDataEntries(body) : null
}

/**
 * Wraps a fetch so a FormData body holding a native `{ uri, name, type }` file
 * part is sent as an encoded multipart payload. Every other request — including
 * a FormData of only strings and Blobs — reaches `fetchImpl` untouched.
 */
export function withNativeFileParts(
    fetchImpl: Fetch,
    readers: MultipartReaders,
    entriesOf: (body: BodyInit | null | undefined) => FormDataEntries | null = bodyEntries
): Fetch {
    return async (url, config) => {
        const entries = entriesOf(config?.body)
        if (!entries?.some(([, value]) => isNativeFilePart(value))) {
            return fetchImpl(url, config)
        }
        const { body, contentType } = await encodeMultipart(entries, readers)
        const headers = new Headers(config?.headers)
        headers.set('Content-Type', contentType)
        return fetchImpl(url, { ...config, body, headers })
    }
}
