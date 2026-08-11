/**
 * Resolving image sources that point at protected PocketBase files.
 *
 * The STORED src is root-relative and tokenless — `/api/files/<collection>/
 * <recordId>/<storedFile>` — because it persists into markdown and a shared
 * Y.Doc. A baked-in file token is per-user and expires within the hour, so it
 * would leak one user's token to every collaborator and then rot; an absolute
 * URL bakes in a server host that can change (the same reason help bodies
 * write `{{server-host}}`). Render surfaces call resolveProtectedFileSrc with
 * a fresh token instead. (text/ established the tokenless rule but stores
 * absolute URLs; the resolver accepts both forms so content stays portable.)
 */

const FILES_PATH = '/api/files/'

/**
 * True for srcs this module would rewrite: a root-relative PocketBase file
 * path, or an absolute URL whose path is one (the origin check against the
 * actual server happens in resolveProtectedFileSrc, which is what appends the
 * token). data:/blob: payloads and already-tokened URLs are never touched.
 */
export function isProtectedFileSrc(src: string): boolean {
    if (!src || src.includes('token=')) return false
    if (src.startsWith(FILES_PATH)) return true
    return tryParseUrl(src)?.pathname.startsWith(FILES_PATH) ?? false
}

/**
 * Absolutize a protected file src against the server base URL and append a
 * fresh file token.
 *
 * Returns the src unchanged when it is not a protected file path — or when it
 * is an absolute URL whose origin differs from (or cannot be checked against)
 * the server's, where appending our token would hand it to a foreign host. A
 * relative base URL (web dev serves PocketBase same-origin as `/`) leaves a
 * relative src relative; the browser resolves it, and only the token is added.
 */
export function resolveProtectedFileSrc(
    src: string,
    baseURL: string,
    token: string | undefined
): string {
    if (!isProtectedFileSrc(src)) return src
    const base = tryParseUrl(baseURL)

    if (src.startsWith(FILES_PATH)) {
        const absolute = base ? new URL(src, base).toString() : src
        return appendToken(absolute, token)
    }

    const parsed = tryParseUrl(src)
    if (!parsed || !base || parsed.origin !== base.origin) return src
    return appendToken(src, token)
}

function appendToken(url: string, token: string | undefined): string {
    if (!token) return url
    return `${url}${url.includes('?') ? '&' : '?'}token=${encodeURIComponent(token)}`
}

function tryParseUrl(value: string): URL | null {
    try {
        return new URL(value)
    } catch {
        return null
    }
}
