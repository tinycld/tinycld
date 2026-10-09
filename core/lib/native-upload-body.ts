import type { Fetch } from '@tinycld/core/lib/read-only-retry'

// Web: the browser's FormData only ever holds strings and Blobs, which the
// browser's fetch encodes itself, so requests pass through unchanged. The
// native build resolves native-upload-body.native.ts instead.
export function withNativeUploadBodies(fetchImpl: Fetch): Fetch {
    return fetchImpl
}
