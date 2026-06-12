import { PB_SERVER_ADDR } from '@tinycld/core/lib/config'
import { useEffect, useRef, useState } from 'react'

export interface ProgressStep {
    step: string
    progress: number
    message: string
}

export type OperationStatus = 'running' | 'success' | 'failed'

export interface InstallProgress {
    steps: ProgressStep[]
    status: OperationStatus
    error: string | null
}

type TerminalStatus = Exclude<OperationStatus, 'running'>

const POLL_INTERVAL_MS = 2_000

// The durable job-status poll only needs to run near the END of an operation —
// the window where the SSE stream can't deliver the verdict. We arm it once the
// live stream has carried the bar to within this band of the finish (the restart
// that drops the stream always follows the final pre-restart steps), or
// immediately if the stream dies. Polling the whole multi-minute build is wasted
// requests: the stream covers progress fine until the restart seam.
const POLL_ARM_PROGRESS = 90

// useInstallProgress tracks a background package operation from two sources that
// each cover the other's blind spot:
//
//   - the SSE event stream drives the LIVE progress bar + step log, but it can't
//     survive the server restart that every successful apply ends with — the new
//     process has no in-memory job, so the EventSource reconnect 404s;
//   - the job-status endpoint (backed by pkg_install_log, finalized BEFORE the
//     restart) is the durable TERMINAL truth, polled by the unique job id so a
//     re-run for the same package can never resolve against a stale row.
//
// The poll is the safety net for the restart seam, not a parallel live source, so
// it's DEFERRED: it starts only once the stream has reached POLL_ARM_PROGRESS or
// the stream has errored out (the restart killed it). Until then the SSE stream
// alone drives the UI, so the early/middle stages don't double up requests.
//
// Whichever source reports a terminal state first resolves the modal; later
// reports are ignored. Returns the accumulated steps + resolved status + error.
export function useInstallProgress(
    isActive: boolean,
    jobId: string | null,
    authToken: string,
    onSuccess: () => void
): InstallProgress {
    const [steps, setSteps] = useState<ProgressStep[]>([])
    const [status, setStatus] = useState<OperationStatus>('running')
    const [error, setError] = useState<string | null>(null)
    // Set once the stream dies (restart) — arms the poll AND is itself a poll
    // trigger, distinct from the progress-band arm so a stream that drops before
    // 90% still starts the poll promptly.
    const [streamGone, setStreamGone] = useState(false)

    // Keep the success callback fresh without retriggering the source effects
    // when the caller passes a new closure each render. The ref read inside
    // resolve() always sees the latest callback.
    const onSuccessRef = useRef(onSuccess)
    onSuccessRef.current = onSuccess

    // resolve is recreated each render but only ever reads stable setters + the
    // ref, so passing it to the effects below adds no real dependency churn. It
    // applies the FIRST terminal outcome and is a no-op afterward (status leaves
    // 'running' exactly once).
    const resolve = useRef((resolved: TerminalStatus, errMsg?: string) => {
        setStatus(prev => {
            if (prev !== 'running') return prev
            if (resolved === 'success') onSuccessRef.current()
            return resolved
        })
        if (resolved === 'failed') {
            setError(prev => prev ?? errMsg ?? 'The operation failed — check the server logs.')
        }
    }).current

    useEffect(() => {
        if (!isActive || !jobId) return
        setSteps([])
        setStatus('running')
        setError(null)
        setStreamGone(false)
    }, [isActive, jobId])

    useEffect(() => {
        if (!isActive || !jobId) return
        return subscribeProgressStream(jobId, authToken, {
            onStep: step => setSteps(prev => [...prev, step]),
            onResolved: resolve,
            onStreamGone: () => setStreamGone(true),
        })
    }, [isActive, jobId, authToken, resolve])

    // Arm the durable poll only at the restart seam: once the live bar crosses
    // POLL_ARM_PROGRESS, or the stream has dropped. Before that the SSE stream is
    // the sole source, so we don't poll job-status during the long build.
    const maxProgress = steps.length > 0 ? steps[steps.length - 1].progress : 0
    const pollArmed = streamGone || maxProgress >= POLL_ARM_PROGRESS

    useEffect(() => {
        if (!isActive || !jobId || !pollArmed) return
        return pollJobOutcome(jobId, authToken, resolve)
    }, [isActive, jobId, authToken, resolve, pollArmed])

    return { steps, status, error }
}

interface StreamHandlers {
    onStep: (step: ProgressStep) => void
    onResolved: (status: TerminalStatus, error?: string) => void
    // Fired once when the stream is truly CLOSED before delivering a terminal
    // event — i.e. the restart (or a genuine drop) killed it. It does NOT resolve
    // the modal; it hands off to the durable job-status poll, which is the
    // authoritative arbiter of success/failure across the restart seam.
    onStreamGone: () => void
}

// subscribeProgressStream wires the SSE EventSource to the live progress
// handlers and returns an unsubscribe. A transient drop auto-reconnects (the
// server replays history + any terminal event). A connection that's truly CLOSED
// before any terminal event hands off to the poll via onStreamGone rather than
// resolving — every successful install-class op ends by restarting the server,
// which drops the stream, so a closed stream is the EXPECTED path to the verdict,
// not a failure. The job-status poll then reports the real outcome.
function subscribeProgressStream(
    jobId: string,
    authToken: string,
    handlers: StreamHandlers
): () => void {
    const url = `${PB_SERVER_ADDR}/api/admin/packages/events/${jobId}?token=${encodeURIComponent(authToken)}`
    const source = new EventSource(url)
    let terminal = false

    source.addEventListener('progress', (event: MessageEvent) => {
        handlers.onStep(JSON.parse(event.data) as ProgressStep)
    })

    source.addEventListener('complete', (event: MessageEvent) => {
        const data = JSON.parse(event.data) as { status: string; error?: string }
        terminal = true
        source.close()
        handlers.onResolved(data.status === 'success' ? 'success' : 'failed', data.error)
    })

    source.addEventListener('error', () => {
        // Ignore reconnect blips (CONNECTING/OPEN) and any post-terminal close —
        // a stalled-but-open stream is indistinguishable from "still running", so
        // acting early would freeze the bar. A genuinely CLOSED stream means the
        // restart dropped it: arm the poll (onStreamGone) and let it arbitrate,
        // rather than resolving 'failed' here and racing the poll's success.
        if (terminal || source.readyState !== EventSource.CLOSED) return
        terminal = true
        handlers.onStreamGone()
    })

    return () => source.close()
}

// pollJobOutcome polls the durable job-status endpoint until the operation
// reaches a terminal state, then reports it. Keyed by the unique job id, so it
// needs no stale-row guarding. Connection errors mid-restart are expected and
// simply retried. Returns a cancel function.
function pollJobOutcome(
    jobId: string,
    authToken: string,
    onResolved: (status: TerminalStatus, error?: string) => void
): () => void {
    let cancelled = false
    const url = `${PB_SERVER_ADDR}/api/admin/packages/job-status/${jobId}`

    async function readOnce(): Promise<boolean> {
        let res: Response
        try {
            res = await fetch(url, { headers: { Authorization: authToken } })
        } catch {
            return false // server mid-restart — retry
        }
        if (!res.ok) return false
        const body = (await res.json().catch(() => null)) as {
            status?: string
            error?: string
        } | null
        if (body?.status === 'success') {
            onResolved('success')
            return true
        }
        if (body?.status === 'failed') {
            onResolved('failed', body.error)
            return true
        }
        return false
    }

    async function loop() {
        while (!cancelled) {
            if (await readOnce()) return
            await new Promise(r => setTimeout(r, POLL_INTERVAL_MS))
        }
    }
    loop()

    return () => {
        cancelled = true
    }
}
