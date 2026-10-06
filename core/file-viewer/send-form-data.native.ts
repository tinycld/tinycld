import { sendNativeMultipart } from '@tinycld/core/lib/native-upload.native'
import { formDataEntries, requireUploadMethod } from '@tinycld/core/lib/streamed-upload'
import type { SendFormDataParams, SendFormDataResult } from './send-form-data-types'

// Native: the shared native uploader (core/lib/native-upload.native.ts), which
// streams files from disk and reports progress, instead of React Native's
// XMLHttpRequest.

async function parsedBody(response: Response): Promise<unknown> {
    const text = await response.text()
    try {
        return text ? JSON.parse(text) : null
    } catch {
        // Non-JSON response — treat as an empty success body, as on web.
        return null
    }
}

/** One upload attempt. Rejects only on transport failure or abort; an HTTP
 * error status resolves normally so the caller can inspect it. */
export async function sendFormDataOnce(params: SendFormDataParams): Promise<SendFormDataResult> {
    const { url, formData, authToken, method, onProgress, signal } = params
    const response = await sendNativeMultipart({
        url,
        method: requireUploadMethod(method),
        headers: authToken ? { Authorization: authToken } : {},
        entries: formDataEntries(formData),
        onProgress,
        signal,
    })
    return {
        status: response.status,
        body: await parsedBody(response),
        retryAfter: response.headers.get('Retry-After'),
    }
}
