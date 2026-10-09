import { captureException } from '@tinycld/core/lib/errors'
import { reloadLiveStores } from '@tinycld/core/lib/pocketbase'
import { createDebouncedTrigger, isReconnect, isResume } from '@tinycld/core/lib/reload-on-resume'
import { useConnectivityStore } from '@tinycld/core/lib/stores/connectivity-store'
import { useEffect } from 'react'
import { AppState } from 'react-native'

const RESUME_RELOAD_DEBOUNCE_MS = 1000

/**
 * Reloads every live pbtsdb collection when the app returns to the foreground
 * or comes back online. pbtsdb refetches on its own only after a realtime
 * reconnect the server did not resume; a backgrounded app or a dropped network
 * can miss changes with no reconnect at all, and its failed loads wait out
 * their retry backoff. A reload refetches at once and wakes those waiting
 * retries.
 *
 * AppState covers both platforms (react-native-web derives it from page
 * visibility). Coming back online is read from the connectivity store, which
 * the web `online` event and native NetInfo both feed (useConnectivityDetector).
 */
export function useReloadOnResume(): void {
    useEffect(() => {
        const reload = createDebouncedTrigger(reloadLiveStores, RESUME_RELOAD_DEBOUNCE_MS, err =>
            captureException('pbtsdb.reloadOnResume', err)
        )
        let appState: string = AppState.currentState
        const appStateSubscription = AppState.addEventListener('change', next => {
            if (isResume(appState, next)) reload.trigger()
            appState = next
        })
        const unsubscribeConnectivity = useConnectivityStore.subscribe((state, previous) => {
            if (isReconnect(previous, state)) reload.trigger()
        })
        return () => {
            appStateSubscription.remove()
            unsubscribeConnectivity()
            reload.cancel()
        }
    }, [])
}
