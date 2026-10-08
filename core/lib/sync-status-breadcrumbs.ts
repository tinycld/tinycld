import type { SyncStatus } from 'pbtsdb'

export interface SyncBreadcrumb {
    message: string
    extra?: Record<string, unknown>
}

/**
 * The breadcrumbs one sync-status change is worth: a change of realtime state,
 * and loads starting or stopping retrying. A new retry time within the same
 * state, or a changed count of retrying loads, is not — those would bury a
 * crash report's trail in noise.
 */
export function syncBreadcrumbs(previous: SyncStatus, next: SyncStatus): SyncBreadcrumb[] {
    const crumbs: SyncBreadcrumb[] = []
    if (previous.realtime.state !== next.realtime.state) {
        crumbs.push(
            next.realtime.state === 'reconnecting'
                ? { message: 'reconnecting', extra: { attempt: next.realtime.attempt } }
                : { message: next.realtime.state }
        )
    }
    const wasRetrying = previous.loads.retrying > 0
    const isRetrying = next.loads.retrying > 0
    if (isRetrying && !wasRetrying) {
        crumbs.push({ message: 'loads retrying', extra: { retrying: next.loads.retrying } })
    } else if (wasRetrying && !isRetrying) {
        crumbs.push({ message: 'loads recovered' })
    }
    return crumbs
}
