import { withNativeUploadBodies } from '@tinycld/core/lib/native-upload-body'
import { type Fetch, withReadOnlyRetry } from '@tinycld/core/lib/read-only-retry'

// The one fetch every call to OUR server should go through. It has the same
// signature as the platform `fetch`, so it drops into any existing call site
// unchanged, and it retries a briefly-unavailable server the same way
// pbtsdb's REST calls do (see read-only-retry.ts) — without this, a direct
// `fetch` call sees the 503 once and fails where a pbtsdb-backed read or
// write would have quietly recovered.
//
// On native it also encodes React Native `{ uri, name, type }` file parts
// itself, because the native global fetch (expo/fetch) rejects them; see
// multipart-body.ts. The encoding runs once, outside the retry, so a retried
// upload resends the same bytes instead of reading the file again.
//
// `globalThis.fetch` is read per call, not captured at module load, so a test
// or polyfill that replaces it later still takes effect.
export const serverFetch: Fetch = withNativeUploadBodies(
    withReadOnlyRetry((url, config) => fetch(url, config))
)
