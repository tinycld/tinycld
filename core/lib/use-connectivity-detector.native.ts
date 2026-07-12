import NetInfo from '@react-native-community/netinfo'
import { getResolvedAddress, probe } from '@tinycld/core/lib/server-address'
import { useConnectivityStore } from '@tinycld/core/lib/stores/connectivity-store'
import { useEffect } from 'react'

const OFFLINE_DEBOUNCE_MS = 1500
const SERVER_CONFIRM_TIMEOUT_MS = 3000

export function useConnectivityDetector(): void {
    useEffect(() => {
        const { setOnline } = useConnectivityStore.getState()
        let offlineTimer: ReturnType<typeof setTimeout> | null = null
        let cancelled = false

        const clearOfflineTimer = () => {
            if (offlineTimer) {
                clearTimeout(offlineTimer)
                offlineTimer = null
            }
        }

        // NetInfo's isInternetReachable is a probe to a public host, which
        // returns false on networks that can't reach it even when OUR server
        // can — a captive/filtered LAN, or the iOS simulator (whose reachability
        // probe is unreliable and routinely reports offline while localhost:7100
        // answers fine). Treating that as offline pops the full-screen blocking
        // OfflineOverlay over a perfectly usable app. So before flipping offline,
        // confirm against the server we actually depend on: if /api/health still
        // answers, we're online regardless of what NetInfo thinks of the wider
        // internet. Only when the server probe ALSO fails do we go offline.
        const confirmOfflineAgainstServer = async () => {
            const address = getResolvedAddress()
            if (!address) {
                if (!cancelled) setOnline(false)
                return
            }
            try {
                await probe(address, SERVER_CONFIRM_TIMEOUT_MS)
                // Server reachable — NetInfo's "offline" is a false negative.
                if (!cancelled) setOnline(true)
            } catch {
                if (!cancelled) setOnline(false)
            }
        }

        const unsubscribe = NetInfo.addEventListener(state => {
            const reachable = state.isInternetReachable !== false
            const online = Boolean(state.isConnected) && reachable

            if (online) {
                clearOfflineTimer()
                setOnline(true)
                return
            }

            if (offlineTimer) return
            offlineTimer = setTimeout(() => {
                offlineTimer = null
                void confirmOfflineAgainstServer()
            }, OFFLINE_DEBOUNCE_MS)
        })

        return () => {
            cancelled = true
            unsubscribe()
            clearOfflineTimer()
        }
    }, [])
}
