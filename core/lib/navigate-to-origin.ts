import { trace } from '@tinycld/core/lib/debug-trace'

// Navigating to a DIFFERENT server's origin — a full page load, not an in-app
// route. Each server is its own deployment, database and client bundle, so
// there is no route from one to another; the saved-server switcher
// (lib/use-saved-servers.ts) hands us the target origin and we leave.
//
// No-op outside the browser: on native the switcher re-points the app at the
// new address in place rather than navigating a document.
export function navigateToOrigin(url: string): void {
    trace('navigateToOrigin assign', { url })
    if (typeof window !== 'undefined') window.location.assign(url)
}
