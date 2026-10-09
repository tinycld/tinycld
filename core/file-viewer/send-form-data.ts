import type { SendFormDataParams, SendFormDataResult } from './send-form-data-types'

// Web: XMLHttpRequest, the one browser API that reports upload progress.
// send-form-data.native.ts is the native variant.

/** One upload attempt. Rejects only on transport failure or abort; an HTTP
 * error status resolves normally so the caller can inspect it (e.g. to
 * detect a read-only pause) before deciding whether to retry. */
export function sendFormDataOnce(params: SendFormDataParams): Promise<SendFormDataResult> {
    const { url, formData, authToken, method, onProgress, signal } = params

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
            resolve({
                status: xhr.status,
                body: parsed,
                retryAfter: xhr.getResponseHeader('Retry-After'),
            })
        }
        xhr.onerror = () => reject(new TypeError('Network request failed'))
        xhr.onabort = () => reject(new DOMException('Aborted', 'AbortError'))

        signal?.addEventListener('abort', () => xhr.abort(), { once: true })

        xhr.send(formData)
    })
}
