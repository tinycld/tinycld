// Keys whose *value* is PII or a credential. The credential group
// (token/password/secret/authorization/auth/apikey) was added because query
// errors and fetch breadcrumbs carry secrets — e.g. a `['share-session', token]`
// queryKey or a `?token=...` URL — that were previously shipped to Sentry raw.
// `key` is anchored (word-boundary-ish) so it scrubs `apiKey`/`api_key`/`s3_key`
// without swallowing benign identifiers like `orgId` or `monkey`.
export const PII_KEY_PATTERN =
    /email|body|subject|name|phone|address|content|filename|title|token|password|passwd|secret|authorization|auth|apikey|api_key|(?:^|[_-])key(?:$|[_-])|^key$/i

// Credential query-param names whose value must be redacted inside a URL/string,
// e.g. `https://x/y?token=abc&sig=zzz` -> `?token=[Filtered]&sig=[Filtered]`.
const CREDENTIAL_PARAM_PATTERN =
    /([?&#](?:token|access_token|refresh_token|password|passwd|secret|authorization|auth|api[_-]?key|key|sig|signature|code|session)=)([^&#\s]+)/gi

// queryKey arrays put the secret in a positional slot next to a marker string,
// e.g. `['share-session', '<token>']`. Redact the element(s) following such a
// marker rather than relying on a key name (there is none in an array).
const CREDENTIAL_MARKER_PATTERN =
    /(?:^|[_-])(?:session|token|secret|password|auth|apikey|api_key)(?:$|[_-])/i

const FILTERED = '[Filtered]'
const CIRCULAR = '[Circular]'

function scrubCredentialsInString(value: string): string {
    return value.replace(
        CREDENTIAL_PARAM_PATTERN,
        (_match, prefix: string) => `${prefix}${FILTERED}`
    )
}

function scrubArray(value: unknown[], seen: WeakSet<object>): unknown[] {
    let redactNext = false
    return value.map(item => {
        if (redactNext && typeof item === 'string') {
            redactNext = false
            return FILTERED
        }
        redactNext = typeof item === 'string' && CREDENTIAL_MARKER_PATTERN.test(item)
        return scrubPII(item, seen)
    })
}

export function scrubPII<T>(value: T, seen: WeakSet<object> = new WeakSet()): T {
    if (value === null || value === undefined) return value
    if (typeof value === 'string') return scrubCredentialsInString(value) as unknown as T
    if (typeof value !== 'object') return value

    const obj = value as unknown as object
    if (seen.has(obj)) {
        return CIRCULAR as unknown as T
    }
    seen.add(obj)

    if (Array.isArray(value)) {
        return scrubArray(value, seen) as unknown as T
    }

    const out: Record<string, unknown> = {}
    for (const [key, inner] of Object.entries(value as Record<string, unknown>)) {
        if (PII_KEY_PATTERN.test(key)) {
            out[key] = FILTERED
        } else {
            out[key] = scrubPII(inner, seen)
        }
    }
    return out as unknown as T
}
