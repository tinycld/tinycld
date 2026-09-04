import { pb } from '../lib/pocketbase'
import type { PickedFile } from './picked-file'

/**
 * Multipart upload with progress, for callers that need a progress bar.
 *
 * This is deliberately XMLHttpRequest rather than `fetch`: only XHR exposes
 * `upload.onprogress`, and the PocketBase SDK is built on fetch — so a
 * `collection.create()` with a file field cannot report how far a large upload
 * has got. React Native's XHR polyfill supports upload progress too, so the
 * one code path serves web and native.
 *
 * Bypassing the SDK for file BYTES is the sanctioned exception to the
 * never-bypass-pbtsdb rule; every other read and write stays on pbtsdb. Drive
 * established the exception, and boards and mail now share this implementation
 * rather than each keeping a copy.
 */
export function uploadFormDataWithProgress(params: {
    url: string
    formData: FormData
    authToken: string
    /** Defaults to POST. PATCH is what an update-with-file needs. */
    method?: string
    onProgress?: (loaded: number, total: number) => void
    signal?: AbortSignal
}): Promise<unknown> {
    const { url, formData, authToken, method = 'POST', onProgress, signal } = params

    return new Promise((resolve, reject) => {
        if (signal?.aborted) {
            reject(new DOMException('Aborted', 'AbortError'))
            return
        }

        const xhr = new XMLHttpRequest()
        xhr.open(method, url, true)
        if (authToken) {
            xhr.setRequestHeader('Authorization', authToken)
        }

        if (onProgress) {
            xhr.upload.onprogress = e => {
                if (e.lengthComputable) onProgress(e.loaded, e.total)
            }
        }

        xhr.onload = () => {
            const text = typeof xhr.response === 'string' ? xhr.response : xhr.responseText
            let parsed: unknown = null
            try {
                parsed = text ? JSON.parse(text) : null
            } catch {
                // Non-JSON response — treat as an empty success body.
                parsed = null
            }
            if (xhr.status >= 200 && xhr.status < 300) {
                resolve(parsed)
            } else {
                const message =
                    parsed &&
                    typeof parsed === 'object' &&
                    'message' in parsed &&
                    typeof parsed.message === 'string'
                        ? parsed.message
                        : `Upload failed (${xhr.status})`
                reject(new Error(message))
            }
        }
        xhr.onerror = () => reject(new TypeError('Network request failed'))
        xhr.onabort = () => reject(new DOMException('Aborted', 'AbortError'))

        signal?.addEventListener('abort', () => xhr.abort(), { once: true })

        xhr.send(formData)
    })
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
 * Creates one record carrying one file.
 *
 * `PickedFile.file` is only a real `File` on web — on native it is a
 * `{ uri, name, type, size }` shape that RN's FormData polyfill understands.
 * It is therefore appended straight into FormData and never inspected.
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
