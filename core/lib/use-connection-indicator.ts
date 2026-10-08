import { useAuth } from '@tinycld/core/lib/auth'
import {
    CONNECTION_INDICATOR_DELAY_MS,
    type ConnectionIndicatorState,
    connectionNotice,
} from '@tinycld/core/lib/connection-indicator'
import { pb } from '@tinycld/core/lib/pocketbase'
import { isRealtimeEnabled } from '@tinycld/core/lib/realtime-enabled'
import { useConnectivityStore } from '@tinycld/core/lib/stores/connectivity-store'
import { useSyncStatus } from 'pbtsdb'
import { useEffect, useState } from 'react'

/**
 * What the connection notice shows; see connectionNotice for the states.
 *
 * Sources: the connectivity store (`isOnline` from the web online/offline
 * events and native NetInfo; `isServerReachable` from sustained request
 * failures and the health check) and pbtsdb's `useSyncStatus` (live-update
 * stream reconnecting, loads retrying). `pb` is a module singleton for the
 * life of a JS context — a server switch restarts the context — so the
 * imported client is always the current one.
 *
 * A problem shows only after it has lasted CONNECTION_INDICATOR_DELAY_MS, so a
 * blip never flashes the notice; recovery hides it at once. A reconnect that
 * lasts CONNECTION_ESCALATE_AFTER_MS turns into `unreachable`, which offers
 * options. Hidden for a signed-out user and on a page with realtime turned off.
 */
export function useConnectionIndicator(): ConnectionIndicatorState {
    const { user } = useAuth({ throwIfAnon: false })
    const isOnline = useConnectivityStore(s => s.isOnline)
    const isServerReachable = useConnectivityStore(s => s.isServerReachable)
    const sync = useSyncStatus(pb)

    // The clock the notice is evaluated against, advanced by the timer below
    // when the delay ends or an escalation is due: local, timer-driven state.
    const [now, setNow] = useState(() => Date.now())
    const notice = connectionNotice(
        {
            isOnline,
            isServerReachable,
            isSignedIn: !!user,
            isRealtimeEnabled: isRealtimeEnabled(),
            sync,
        },
        now
    )
    const isProblem = notice.state !== 'hidden'

    const [problemSince, setProblemSince] = useState<number | null>(null)
    useEffect(() => {
        const at = Date.now()
        setProblemSince(isProblem ? at : null)
        setNow(at)
    }, [isProblem])

    const showsAt = problemSince === null ? null : problemSince + CONNECTION_INDICATOR_DELAY_MS
    const pendingShow = showsAt !== null && now < showsAt ? showsAt : null
    const wakeAt = earliest(pendingShow, notice.escalatesAt)
    useEffect(() => {
        if (wakeAt === null) return
        const timer = setTimeout(() => setNow(Date.now()), Math.max(0, wakeAt - Date.now()))
        return () => clearTimeout(timer)
    }, [wakeAt])

    const isVisible = isProblem && showsAt !== null && now >= showsAt
    return isVisible ? notice.state : 'hidden'
}

function earliest(a: number | null, b: number | null): number | null {
    if (a === null) return b
    if (b === null) return a
    return Math.min(a, b)
}
