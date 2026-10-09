import { eq } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useEffect, useRef } from 'react'

export interface ProgressStep {
    step: string
    progress: number
    message: string
    // The step's own completion (0-100), sent only by a step that can measure it.
    stepProgress?: number
}

export type OperationStatus = 'running' | 'success' | 'failed'

export interface InstallProgress {
    steps: ProgressStep[]
    status: OperationStatus
    error: string | null
}

// useInstallProgress tracks a background package operation by watching its
// pkg_install_log row — the SAME durable source whichever side of the
// server's exit-75 restart you're on, so there is no separate live-vs-restart
// path to reconcile. pbtsdb reconnects and reloads live queries once the new
// process answers again, so the row simply keeps updating straight through
// the seam: the throttled `steps`/`current_step` writes carry progress while
// the job runs, and `status`/`error` carry the terminal outcome once it ends.
//
// This replaced an EventSource stream authenticated by a `?token=<JWT>` query
// param (browser EventSource can't send headers) plus a durable-poll fallback
// for the restart seam — the token leaked an admin credential into server
// logs, and the fallback was a second source to keep in sync with the first.
// A live query needs neither: it carries one admin-authenticated connection
// the normal way, and it IS the durable source, not a fallback to one.
export function useInstallProgress(
    isActive: boolean,
    jobId: string | null,
    onSuccess: () => void
): InstallProgress {
    const [pkgInstallLogCollection] = useStore('pkg_install_log')
    const { data: row } = useLiveQuery(query =>
        isActive && jobId
            ? query
                  .from({ log: pkgInstallLogCollection })
                  .where(({ log }) => eq(log.job_id, jobId))
                  .findOne()
            : null
    )

    // Keep the success callback fresh without retriggering the effect below
    // when the caller passes a new closure each render.
    const onSuccessRef = useRef(onSuccess)
    onSuccessRef.current = onSuccess

    const status: OperationStatus =
        row?.status === 'success' || row?.status === 'failed' || row?.status === 'rolled_back'
            ? row.status === 'rolled_back'
                ? 'failed'
                : row.status
            : 'running'

    // Fire the success callback exactly once per job when the row resolves.
    useEffect(() => {
        if (status === 'success') onSuccessRef.current()
    }, [status])

    const steps = (row?.steps as ProgressStep[] | undefined) ?? []
    const error = status === 'failed' ? row?.error || rolledBackMessage(row?.status) : null

    return { steps, status, error }
}

// 'rolled_back' is the server's terminal status after a post-activation
// health-check rollback — distinct from a plain 'failed' so the UI can say
// what actually happened when the row carries no error string of its own.
function rolledBackMessage(status: string | undefined): string | null {
    if (status !== 'rolled_back') return null
    return 'The update failed its health check and was rolled back.'
}
