import { isUploadFile, sendNativeMultipart } from '@tinycld/core/lib/native-upload.native'
import type { Fetch } from '@tinycld/core/lib/read-only-retry'
import { withStreamedUploads } from '@tinycld/core/lib/streamed-upload'

// A multipart body carrying a file on disk goes to the native uploader, which
// streams it; see native-upload.native.ts.
export function withNativeUploadBodies(fetchImpl: Fetch): Fetch {
    return withStreamedUploads(fetchImpl, { isFile: isUploadFile, send: sendNativeMultipart })
}
