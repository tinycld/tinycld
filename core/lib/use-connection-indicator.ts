import { useAuth } from '@tinycld/core/lib/auth'
import {
    CONNECTION_INDICATOR_DELAY_MS,
    type ConnectionIndicatorState,
    connectionIndicatorState,
} from '@tinycld/core/lib/connection-indicator'
import { isRealtimeEnabled } from '@tinycld/core/lib/realtime-enabled'
import { useConnectivityStore } from '@tinycld/core/lib/stores/connectivity-store'
import { useEffect, useState } from 'react'

/**
 * What the connection indicator shows: 'offline' with no network,
 * 'reconnecting' while requests to the server fail (pbtsdb keeps a list
 * loading and retries it through an outage, so without this the user sees a
 * spinner with no reason), and 'hidden' when all is well.
 *
 * A problem shows only after it has lasted CONNECTION_INDICATOR_DELAY_MS, so a
 * blip never flashes the indicator; recovery hides it at once. Hidden for a
 * signed-out user and on a page with realtime turned off (an embed).
 *
 * pbtsdb 2.0 exposes no realtime connection or retry state, so the signals are
 * the network status and tinycld's own record of failing requests (every
 * pbtsdb load goes through `pb.send`, see pocketbase.ts).
 */
export function useConnectionIndicator(): ConnectionIndicatorState {
    const { user } = useAuth({ throwIfAnon: false })
    const isOnline = useConnectivityStore(s => s.isOnline)
    const isServerReachable = useConnectivityStore(s => s.isServerReachable)
    const isRequestFailing = useConnectivityStore(s => s.isRequestFailing)
    const desired = connectionIndicatorState({
        isOnline,
        isServerReachable,
        isRequestFailing,
        isSignedIn: !!user,
        isRealtimeEnabled: isRealtimeEnabled(),
    })

    // The delayed copy of `desired`: local, timer-driven UI state.
    const [shown, setShown] = useState<ConnectionIndicatorState>('hidden')
    useEffect(() => {
        if (desired === 'hidden') {
            setShown('hidden')
            return
        }
        const timer = setTimeout(() => setShown(desired), CONNECTION_INDICATOR_DELAY_MS)
        return () => clearTimeout(timer)
    }, [desired])

    return desired === 'hidden' ? 'hidden' : shown
}
