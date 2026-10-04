import { useQuery } from '@tanstack/react-query'
import { PB_SERVER_ADDR } from '@tinycld/core/lib/config'
import { abortableWait } from '@tinycld/core/lib/read-only-retry'
import { serverFetch } from '@tinycld/core/lib/server-fetch'
import type { OperationStatus } from './use-install-progress'

// 'unconfirmed' covers both a release that did not change within the limit
// and a starting release that could not be read: either way the panel cannot
// tell whether a reload reaches the new server, so it says so.
export type ServerSwitch = 'waiting' | 'ready' | 'unconfirmed'

interface SwitchTiming {
    intervalMs: number
    timeoutMs: number
}

const DEFAULT_TIMING: SwitchTiming = { intervalMs: 2_000, timeoutMs: 120_000 }

// The release id the server answers with now, or '' when it cannot tell (a
// build without a release id, or a request lost while the servers switch).
async function fetchServedReleaseId(): Promise<string> {
    try {
        const res = await serverFetch(`${PB_SERVER_ADDR}/api/version`, { cache: 'no-store' })
        if (!res.ok) return ''
        const body = (await res.json()) as { releaseId?: string }
        return body.releaseId?.trim() ?? ''
    } catch {
        return ''
    }
}

async function waitForNewRelease(
    startRelease: string,
    { intervalMs, timeoutMs }: SwitchTiming,
    signal: AbortSignal
): Promise<Exclude<ServerSwitch, 'waiting'>> {
    if (!startRelease) return 'unconfirmed'
    const deadline = Date.now() + timeoutMs
    for (;;) {
        const served = await fetchServedReleaseId()
        if (served && served !== startRelease) return 'ready'
        if (Date.now() + intervalMs > deadline) return 'unconfirmed'
        await abortableWait(intervalMs, signal)
        if (signal.aborted) throw new Error('setup: stopped waiting for the new server')
    }
}

// useServerSwitch tells the progress panel when a reload reaches the new
// build. A job reports success before the new server is ready, and the old
// server keeps answering until then, so a reload offered at "success" can
// load the previous build again. The release the server answers with when the
// job starts is the reference: /api/version changes only when the new build
// is promoted, which is the moment it takes over from the old server.
export function useServerSwitch(
    isActive: boolean,
    jobId: string | null,
    status: OperationStatus,
    timing: SwitchTiming = DEFAULT_TIMING
): ServerSwitch {
    const isTracking = isActive && jobId !== null

    const startRelease = useQuery({
        queryKey: ['served-release', 'start', jobId],
        queryFn: fetchServedReleaseId,
        enabled: isTracking,
        staleTime: Number.POSITIVE_INFINITY,
        retry: false,
    })

    const outcome = useQuery({
        queryKey: ['served-release', 'switch', jobId, startRelease.data],
        queryFn: ({ signal }) => waitForNewRelease(startRelease.data ?? '', timing, signal),
        enabled: isTracking && status === 'success' && startRelease.isSuccess,
        staleTime: Number.POSITIVE_INFINITY,
        retry: false,
    })

    return outcome.data ?? 'waiting'
}
