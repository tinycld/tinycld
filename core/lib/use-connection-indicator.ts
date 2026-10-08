import { useAuth } from '@tinycld/core/lib/auth'
import {
    CONNECTION_INDICATOR_DELAY_MS,
    type ConnectionIndicatorState,
    connectionIndicatorState,
} from '@tinycld/core/lib/connection-indicator'
import { pb } from '@tinycld/core/lib/pocketbase'
import { isRealtimeEnabled } from '@tinycld/core/lib/realtime-enabled'
import { useConnectivityStore } from '@tinycld/core/lib/stores/connectivity-store'
import { useSyncStatus } from 'pbtsdb'
import { useEffect, useState } from 'react'

/**
 * What the connection indicator shows: 'offline' with no network (the
 * connectivity store: web online/offline events, native NetInfo),
 * 'reconnecting' while pbtsdb reports its live-update stream reconnecting or
 * a list's load retrying (`useSyncStatus`), and 'hidden' when all is well.
 * See connectionIndicatorState for exactly which states count.
 *
 * A problem shows only after it has lasted CONNECTION_INDICATOR_DELAY_MS, so a
 * blip never flashes the indicator; recovery hides it at once. Hidden for a
 * signed-out user and on a page with realtime turned off (an embed).
 *
 * `pb` is a module singleton for the life of a JS context: a server switch
 * restarts the context (switch-server.ts), so the imported client is always
 * the current one.
 */
export function useConnectionIndicator(): ConnectionIndicatorState {
    const { user } = useAuth({ throwIfAnon: false })
    const isOnline = useConnectivityStore(s => s.isOnline)
    const sync = useSyncStatus(pb)
    const desired = connectionIndicatorState({
        isOnline,
        isSignedIn: !!user,
        isRealtimeEnabled: isRealtimeEnabled(),
        sync,
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
