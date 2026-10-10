import { captureException } from '@tinycld/core/lib/errors'
import { reloadLiveStores } from '@tinycld/core/lib/pocketbase'
import { reconnectRealtimeNow } from '@tinycld/core/lib/realtime-enabled'
import {
    createAwayClock,
    createCatchUp,
    createDebouncedTrigger,
    isBackground,
    isDisconnect,
    isInTouch,
    isReconnect,
    isResume,
} from '@tinycld/core/lib/reload-on-resume'
import { useConnectivityStore } from '@tinycld/core/lib/stores/connectivity-store'
import { useEffect } from 'react'
import { AppState, Platform } from 'react-native'

const RESUME_RELOAD_DEBOUNCE_MS = 1000

/**
 * Catches the app up when it returns to the foreground or comes back online.
 *
 * After a short absence the realtime connection catches up on its own: it
 * reconnects (at once, instead of after its backoff) and the server replays
 * the events it missed. When the server cannot replay them, pbtsdb reloads.
 *
 * On native, after RELOAD_AFTER_AWAY_MS (reload-on-resume.ts) or more, every
 * live pbtsdb collection reloads as well: the OS suspends a backgrounded app,
 * whose connection can then die with no error, so it misses changes with no
 * reconnect at all; its failed loads also wait out their retry backoff, and a
 * reload wakes them. Web does not reload: a hidden tab keeps running and keeps
 * its connection, and reloading every query on each return to the tab costs
 * more than it catches.
 *
 * AppState covers both platforms (react-native-web derives it from page
 * visibility). Online state is read from the connectivity store, which the web
 * `online` event and native NetInfo both feed (useConnectivityDetector).
 */
export function useReloadOnResume(): void {
    useEffect(() => {
        const away = createAwayClock()
        const catchUp = createCatchUp({
            inTouch: () => isInTouch(AppState.currentState, useConnectivityStore.getState()),
            away,
            reloadAfterLongAbsence: Platform.OS !== 'web',
            reload: reloadLiveStores,
            reconnect: reconnectRealtimeNow,
        })
        const onReturn = createDebouncedTrigger(catchUp, RESUME_RELOAD_DEBOUNCE_MS, err =>
            captureException('pbtsdb.reloadOnResume', err)
        )
        let appState: string = AppState.currentState
        const appStateSubscription = AppState.addEventListener('change', next => {
            if (isBackground(appState, next)) away.leave()
            if (isResume(appState, next)) onReturn.trigger()
            appState = next
        })
        const unsubscribeConnectivity = useConnectivityStore.subscribe((state, previous) => {
            if (isDisconnect(previous, state)) away.leave()
            if (isReconnect(previous, state)) onReturn.trigger()
        })
        return () => {
            appStateSubscription.remove()
            unsubscribeConnectivity()
            onReturn.cancel()
        }
    }, [])
}
