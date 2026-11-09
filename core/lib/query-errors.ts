import { captureException } from '@tinycld/core/lib/errors'
import { log } from '@tinycld/core/lib/logger'
import { ServerUnreachableError } from '@tinycld/core/lib/server-fetch'
import { ClientResponseError } from 'pocketbase'

// The PocketBase SDK wraps a fetch that rejected as a status-0 response error.
function couldNotReachServer(error: unknown): boolean {
    if (error instanceof ServerUnreachableError) return true
    return (
        error instanceof ClientResponseError &&
        error.originalError instanceof ServerUnreachableError
    )
}

/**
 * A read the server refused for a reason the person caused, which the screen shows
 * as its own state. It is thrown, so the query keeps it as an error state, and it is
 * logged, not reported: it is not a bug.
 */
export class QueryRefusal extends Error {}

// reportQueryError is the global handler for a failed read: useQuery surfaces
// errors as state rather than throwing, so without it a read failure is silent
// (the component shows its own error UI, but nothing is logged). The queryKey
// is the grouping context. It does not import pocketbase.ts, which installs it.
export function reportQueryError(
    error: unknown,
    queryKey: readonly unknown[],
    onSessionEnded: () => void
) {
    if (error instanceof ClientResponseError && error.status === 401) {
        // The server ended the session while the app was open. PocketBase
        // serves a dead token as a guest, so only a request that needs the
        // session fails, and the app stays half signed in until the session
        // is checked again.
        log.info('query.session-ended', 'a query was refused because the session ended', {
            queryKey,
        })
        onSessionEnded()
        return
    }
    if (error instanceof QueryRefusal) {
        log.info('query.refused', error.message, { queryKey })
        return
    }
    if (couldNotReachServer(error)) {
        log.info('query.offline', 'a query could not reach the server', { queryKey })
        return
    }
    captureException('query.error', error, { queryKey })
}
