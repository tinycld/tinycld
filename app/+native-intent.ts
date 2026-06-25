// Maps incoming deep-link / universal-link paths to in-app routes before
// expo-router resolves them. The public web contract is `tinycld.org/demo`
// (what the AASA/assetlinks claim and the marketing button links to), but the
// app's demo handler lives in the pre-auth public tree at `/p/demo`, so we
// rewrite it here. Anything else passes through as a normalized pathname.
export function redirectSystemPath({ path }: { path: string; initial: boolean }): string {
    // `path` may arrive as a bare path (`/demo`) OR as a full URL with ANY scheme:
    // `https://tinycld.org/demo` (universal link) or `tinycld:///` (the app's own
    // custom scheme — what iOS hands a normal home-screen cold launch). The old
    // code only normalized `http`-prefixed strings, so `tinycld:///` fell through
    // verbatim and expo-router tried to match the literal route `tinycld:///` →
    // "Unmatched Route" on every device launch. Normalize any `scheme://…` URL to
    // its pathname so the root launch resolves to `/` (→ app/index.tsx).
    const isSchemeUrl = /^[a-z][a-z0-9+.-]*:\/\//i.test(path)
    let pathname = path
    if (isSchemeUrl) {
        try {
            pathname = new URL(path).pathname || '/'
        } catch {
            // Malformed URL — fall through with the original string.
            return path
        }
    }
    if (pathname === '/demo' || pathname.startsWith('/demo/')) {
        return '/p/demo'
    }
    return pathname
}
