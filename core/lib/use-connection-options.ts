import { captureException } from '@tinycld/core/lib/errors'
import { useMutation } from '@tinycld/core/lib/mutations'
import { CONNECT_HREF } from '@tinycld/core/lib/org-routes'
import { disconnectServer, reloadLiveStores } from '@tinycld/core/lib/pocketbase'
import { retryConnection } from '@tinycld/core/lib/retry-connection'
import { getResolvedAddress, probe } from '@tinycld/core/lib/server-address'
import { useConnectionOptionsStore } from '@tinycld/core/lib/stores/connection-options-store'
import { useConnectivityStore } from '@tinycld/core/lib/stores/connectivity-store'
import { router } from 'expo-router'
import { Platform } from 'react-native'

/**
 * The connection options sheet's state and actions.
 *
 * "Choose another server" is the connect screen's flow, which exists only
 * where the app chose its server: on web the server is the page's own origin.
 */
export function useConnectionOptions() {
    const isOpen = useConnectionOptionsStore(s => s.isOpen)
    const close = useConnectionOptionsStore(s => s.close)
    const address = getResolvedAddress()

    const retry = useMutation({
        mutationFn: () =>
            retryConnection({
                address,
                probe,
                reload: reloadLiveStores,
                setServerReachable: useConnectivityStore.getState().setServerReachable,
                onReloadError: err => captureException('connection.retry.reload', err),
            }),
        onSuccess: close,
        // Shown in the sheet itself ("Still can't reach…"), so no toast.
        onError: () => {},
    })

    // A failed Retry must not still show "Still can't reach…" the next time
    // the options open.
    const closeOptions = () => {
        retry.reset()
        close()
    }

    const chooseServer = async () => {
        closeOptions()
        try {
            // disconnectServer closes realtime and in-flight requests before it
            // drops the address; reconnecting after the address is cleared throws.
            await disconnectServer()
            router.replace(`${CONNECT_HREF}?backTo=/`)
        } catch (err) {
            captureException('connection.choose-server', err)
        }
    }

    return {
        isOpen,
        close: closeOptions,
        address,
        canChooseServer: Platform.OS !== 'web',
        retry: () => retry.mutate(),
        isRetrying: retry.isPending,
        retryFailed: retry.isError,
        chooseServer,
    }
}
