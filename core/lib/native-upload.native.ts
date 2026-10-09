import {
    type MultipartUploadRequest,
    makeBoundary,
    planSingleFileUpload,
    type UploadProgress,
    uploadResultToResponse,
    writeMultipart,
} from '@tinycld/core/lib/streamed-upload'
import { Directory, File, FileMode, Paths, type UploadOptions, UploadType } from 'expo-file-system'

// The app's one native multipart uploader. Both serverFetch (every PocketBase
// SDK request) and uploadFormDataWithProgress (uploads with a progress bar)
// send through it. It uses expo-file-system's upload task, which streams the
// body from disk and reports progress on both platforms:
//
// - iOS (FileSystemUploadTask.swift): MULTIPART writes the body to a temporary
//   file in 64 KB chunks, then URLSession.uploadTask(fromFile:); progress comes
//   from urlSession(_:task:didSendBodyData:...).
// - Android (FileSystemUploadTask.kt): MULTIPART streams the file through an
//   OkHttp MultipartBody, BINARY_CONTENT streams the file as the body; both are
//   wrapped in a CountingRequestBody that emits progress.
//
// `httpMethod` is a free string on both (`request.httpMethod`,
// `Request.Builder.method`), so PocketBase's PATCH updates work.

const CHUNK_BYTES = 1024 * 1024

export function isUploadFile(value: unknown): value is File {
    return value instanceof File
}

// The native uploader writes the file's own name into the part's filename
// without escaping it, so a name that would break the header is made safe.
function safeFileName(name: string): string {
    return name.replace(/["\r\n/\\]/g, '_')
}

// A cache directory for one upload's temporary files, created on first use and
// removed when the upload settles.
function uploadStaging() {
    let directory: Directory | null = null
    return {
        get(): Directory {
            if (!directory) {
                directory = new Directory(
                    Paths.cache,
                    `upload-${Date.now()}-${Math.random().toString(16).slice(2)}`
                )
                directory.create({ intermediates: true, idempotent: true })
            }
            return directory
        },
        remove() {
            if (directory?.exists) directory.delete()
        },
    }
}

// React Native's Blob has no arrayBuffer(); its FileReader reads one from the
// native blob store instead.
function readBlob(blob: Blob): Promise<Uint8Array> {
    if (typeof blob.arrayBuffer === 'function') {
        return blob.arrayBuffer().then(buffer => new Uint8Array(buffer))
    }
    return new Promise((resolve, reject) => {
        const reader = new FileReader()
        reader.onload = () => {
            if (reader.result instanceof ArrayBuffer) resolve(new Uint8Array(reader.result))
            else reject(new Error('FileReader did not return an ArrayBuffer'))
        }
        reader.onerror = () => reject(reader.error ?? new Error('Could not read the Blob'))
        reader.readAsArrayBuffer(blob)
    })
}

async function copyFile(file: File, write: (bytes: Uint8Array) => void): Promise<void> {
    const handle = new File(file.uri).open(FileMode.ReadOnly)
    try {
        const size = handle.size ?? 0
        while ((handle.offset ?? size) < size) write(handle.readBytes(CHUNK_BYTES))
    } finally {
        handle.close()
    }
}

/**
 * Where to send from, and how. One file plus string fields is MULTIPART, from
 * a copy named as the user sees it when the file on disk is named otherwise
 * (a picked photo's UUID). Any other shape — several files, a repeated field,
 * a Blob part — is written to a temporary body file a chunk at a time and sent
 * as BINARY_CONTENT with the multipart Content-Type.
 */
async function prepare(
    request: MultipartUploadRequest,
    staging: () => Directory
): Promise<{ source: File; options: UploadOptions }> {
    const single = planSingleFileUpload(request.entries, isUploadFile)
    if (single) {
        const wantedName = safeFileName(single.file.name)
        let source = new File(single.file.uri)
        if (wantedName !== source.name) {
            const copy = new File(staging(), wantedName)
            await source.copy(copy)
            source = copy
        }
        return {
            source,
            options: {
                uploadType: UploadType.MULTIPART,
                headers: request.headers,
                fieldName: single.fieldName,
                mimeType: single.file.type || undefined,
                parameters: single.parameters,
            },
        }
    }
    const boundary = makeBoundary()
    const body = new File(staging(), 'body')
    body.create()
    const handle = body.open(FileMode.ReadWrite)
    try {
        await writeMultipart(request.entries, boundary, bytes => handle.writeBytes(bytes), {
            isFile: isUploadFile,
            fileName: file => file.name,
            fileType: file => file.type,
            copyFile,
            readBlob,
        })
    } finally {
        handle.close()
    }
    return {
        source: body,
        options: {
            uploadType: UploadType.BINARY_CONTENT,
            headers: {
                ...request.headers,
                'Content-Type': `multipart/form-data; boundary=${boundary}`,
            },
        },
    }
}

function progressListener(onProgress: UploadProgress | undefined) {
    if (!onProgress) return undefined
    return ({ bytesSent, totalBytes }: { bytesSent: number; totalBytes: number }) =>
        onProgress(bytesSent, totalBytes)
}

/**
 * Sends a multipart upload from disk and resolves with the server's response,
 * whatever its status. Rejects with an AbortError when `signal` aborts, and a
 * TypeError when the request could not be made — the way fetch and XHR do.
 */
export async function sendNativeMultipart(request: MultipartUploadRequest): Promise<Response> {
    if (request.signal?.aborted) throw new DOMException('Aborted', 'AbortError')
    const staging = uploadStaging()
    try {
        const { source, options } = await prepare(request, staging.get)
        const result = await source
            .createUploadTask(request.url, {
                ...options,
                httpMethod: request.method,
                onProgress: progressListener(request.onProgress),
                signal: request.signal,
                // A foreground session behaves like fetch and XHR: the transfer
                // starts at once and ends with the JS runtime that awaits it.
                sessionType: 'foreground',
            })
            .uploadAsync()
        return uploadResultToResponse(result, request.url)
    } catch (err) {
        if (request.signal?.aborted) throw new DOMException('Aborted', 'AbortError')
        throw new TypeError('Network request failed', { cause: err })
    } finally {
        staging.remove()
    }
}
