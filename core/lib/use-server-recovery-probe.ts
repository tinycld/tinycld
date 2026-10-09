import { getResolvedAddress, probe } from '@tinycld/core/lib/server-address'
import { useConnectivityStore } from '@tinycld/core/lib/stores/connectivity-store'
import { useEffect } from 'react'

const RECHECK_INTERVAL_MS = 5_000
const PROBE_TIMEOUT_MS = 3_000

/**
 * While the server is marked unreachable and the device has a network, check
 * /api/health every few seconds so the "can't reach" notice clears itself when
 * the server comes back, even if nothing else is making requests. Skipped when
 * the device is offline: those checks could only fail at the radio.
 */
export function useServerRecoveryProbe(): void {
    const isOnline = useConnectivityStore(s => s.isOnline)
    const isServerReachable = useConnectivityStore(s => s.isServerReachable)
    const shouldPoll = isOnline && !isServerReachable

    useEffect(() => {
        if (!shouldPoll) return
        let cancelled = false
        async function check() {
            const address = getResolvedAddress()
            if (!address) return
            try {
                await probe(address, PROBE_TIMEOUT_MS)
                if (!cancelled) useConnectivityStore.getState().setServerReachable(true)
            } catch {
                // Still down; the next tick checks again.
            }
        }
        void check()
        const timer = setInterval(check, RECHECK_INTERVAL_MS)
        return () => {
            cancelled = true
            clearInterval(timer)
        }
    }, [shouldPoll])
}
