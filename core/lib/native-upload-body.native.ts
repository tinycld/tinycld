import type { Fetch } from '@tinycld/core/lib/read-only-retry'
import {
    type StreamedUploadRequest,
    uploadResultToResponse,
    withStreamedUploads,
} from '@tinycld/core/lib/streamed-upload'
import { Directory, File, Paths, UploadType } from 'expo-file-system'

// The native uploader writes the file's own name into the part's filename
// without escaping it, so a name that would break the header is made safe.
function safeFileName(name: string): string {
    return name.replace(/["\r\n/\\]/g, '_')
}

function isFile(value: unknown): value is File {
    return value instanceof File
}

/**
 * Streams a one-file multipart upload from disk with expo-file-system's native
 * uploader: iOS writes the multipart body to a temporary file in 64 KB chunks
 * and hands that file to URLSession's uploadTask(fromFile:); Android streams
 * the file through an OkHttp MultipartBody. The bytes never enter JS.
 */
async function send(request: StreamedUploadRequest<File>): Promise<Response> {
    const { file, fieldName, parameters } = request.upload

    // The uploader names the part after the file on disk, which for a picked
    // photo or a cached download is not the name the user sees. Such a file is
    // sent from a native copy that carries the right name.
    const wantedName = safeFileName(file.name)
    const onDisk = new File(file.uri)
    const staging =
        wantedName === onDisk.name
            ? null
            : new Directory(
                  Paths.cache,
                  `upload-${Date.now()}-${Math.random().toString(16).slice(2)}`
              )
    try {
        let source = onDisk
        if (staging) {
            staging.create({ intermediates: true, idempotent: true })
            source = new File(staging, wantedName)
            await onDisk.copy(source)
        }
        const result = await source.upload(request.url, {
            httpMethod: request.method,
            uploadType: UploadType.MULTIPART,
            headers: request.headers,
            fieldName,
            mimeType: file.type || undefined,
            parameters,
            signal: request.signal,
            // A foreground session behaves like fetch: the transfer starts at
            // once and ends with the JS runtime that awaits it.
            sessionType: 'foreground',
        })
        return uploadResultToResponse(result, request.url)
    } finally {
        if (staging?.exists) staging.delete()
    }
}

export function withNativeUploadBodies(fetchImpl: Fetch): Fetch {
    return withStreamedUploads(fetchImpl, { isFile, send })
}
