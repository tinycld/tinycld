import {
    abortableWait,
    isRetryableBody,
    READ_ONLY_MAX_RETRIES,
    retryAfterMs,
} from '@tinycld/core/lib/read-only-retry'
import { pb } from '../lib/pocketbase'
import type { PickedFile } from './picked-file'
import { sendFormDataOnce } from './send-form-data'

/**
 * Multipart upload with progress, for callers that need a progress bar.
 *
 * The PocketBase SDK is built on fetch, which cannot report upload progress,
 * so a `collection.create()` with a file field cannot say how far a large
 * upload has got. Web sends with XMLHttpRequest (`upload.onprogress`); native
 * sends with expo-file-system's upload task, which streams the file from disk
 * and reports progress (send-form-data.native.ts).
 *
 * Bypassing the SDK for file BYTES is the sanctioned exception to the
 * never-bypass-pbtsdb rule; every other read and write stays on pbtsdb. Drive
 * established the exception, and boards and mail now share this implementation
 * rather than each keeping a copy.
 *
 * This path never touches serverFetch, so it sits outside `pb.beforeSend`'s
 * retry wrapper (read-only-retry.ts) and must retry a briefly-unavailable
 * server itself, using the same predicate and retry budget so an upload pauses
 * and resumes the same way every other write does.
 */
export async function uploadFormDataWithProgress(params: {
    url: string
    formData: FormData
    authToken: string
    /** Defaults to POST. PATCH is what an update-with-file needs. */
    method?: string
    onProgress?: (loaded: number, total: number) => void
    signal?: AbortSignal
}): Promise<unknown> {
    const { url, formData, authToken, method = 'POST', onProgress, signal } = params

    let result = await sendFormDataOnce({ url, formData, authToken, method, onProgress, signal })
    for (let attempt = 0; attempt < READ_ONLY_MAX_RETRIES; attempt++) {
        if (signal?.aborted || !isRetryableBody(result.status, result.body)) break
        await abortableWait(retryAfterMs(result.retryAfter), signal)
        if (signal?.aborted) break
        result = await sendFormDataOnce({ url, formData, authToken, method, onProgress, signal })
    }

    if (result.status >= 200 && result.status < 300) {
        return result.body
    }
    const message =
        result.body &&
        typeof result.body === 'object' &&
        'message' in result.body &&
        typeof result.body.message === 'string'
            ? result.body.message
            : `Upload failed (${result.status})`
    throw new Error(message)
}

export interface UploadRecordParams {
    /** Collection name, e.g. 'boards_attachments'. */
    collection: string
    /**
     * Scalar fields for the record. Pre-generate the id with `newRecordId()`
     * so optimistic UI can reference the row before the response lands.
     */
    fields: Record<string, string>
    file: PickedFile
    /** The file field's name on the collection. Defaults to 'file'. */
    fileField?: string
    onProgress?: (loaded: number, total: number) => void
    signal?: AbortSignal
}

/**
 * Creates one record carrying one file: the browser `File` on web, an
 * expo-file-system `File` on native (see core/lib/upload-file).
 */
export function uploadRecordWithFile(params: UploadRecordParams): Promise<unknown> {
    const { collection, fields, file, fileField = 'file', onProgress, signal } = params

    const formData = new FormData()
    for (const [key, value] of Object.entries(fields)) {
        formData.append(key, value)
    }
    formData.append(fileField, file.file, file.name)

    return uploadFormDataWithProgress({
        url: pb.buildURL(`/api/collections/${encodeURIComponent(collection)}/records`),
        formData,
        authToken: pb.authStore.token ?? '',
        onProgress,
        signal,
    })
}

/**
 * At most one progress write per `intervalMs`, plus an unconditional final
 * one when the last byte lands. Without the final flush a throttled bar
 * stalls a few percent short and never visibly completes; without the
 * throttle a large upload re-renders the surface on every network chunk.
 */
export function throttleProgress(
    write: (loaded: number, total: number) => void,
    intervalMs = 60
): (loaded: number, total: number) => void {
    // Null rather than 0: seeding with 0 makes the FIRST event look like it
    // arrived `Date.now()` ms after a previous one, which is true against a
    // real clock but false at t=0 — and either way the first progress event
    // is the one that turns a bar from empty into moving, so it must never
    // be throttled away.
    let lastUpdate: number | null = null
    return (loaded, total) => {
        const now = Date.now()
        const isFinal = total > 0 && loaded >= total
        if (!isFinal && lastUpdate !== null && now - lastUpdate < intervalMs) return
        lastUpdate = now
        write(loaded, total)
    }
}
