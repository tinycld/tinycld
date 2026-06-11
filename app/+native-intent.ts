// Maps incoming deep-link / universal-link paths to in-app routes before
// expo-router resolves them. The public web contract is `tinycld.org/demo`
// (what the AASA/assetlinks claim and the marketing button links to), but the
// app's demo handler lives in the pre-auth public tree at `/p/demo`, so we
// rewrite it here. Anything else passes through unchanged.
export function redirectSystemPath({ path }: { path: string; initial: boolean }): string {
    try {
        // `path` may arrive as a full URL (https://tinycld.org/demo) or a bare
        // path (/demo), depending on platform and whether it's the cold-start
        // initial URL. Normalize to the pathname.
        const pathname = path.startsWith('http') ? new URL(path).pathname : path
        if (pathname === '/demo') {
            return '/p/demo'
        }
    } catch {
        // Malformed URL — fall through to the original path.
    }
    return path
}
