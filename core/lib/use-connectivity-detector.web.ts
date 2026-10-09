import { useConnectivityStore } from '@tinycld/core/lib/stores/connectivity-store'
import { useEffect } from 'react'

// The store takes the browser's word at once. The connection notice applies
// its own anti-flicker delay (CONNECTION_INDICATOR_DELAY_MS), so a debounce
// here would only add to it: offline would show after both delays, not one.
// A save that starts while the browser is offline cannot reach the server, so
// useWritesAvailable must not wait either.
export function useConnectivityDetector(): void {
    useEffect(() => {
        if (typeof window === 'undefined') return

        const { setOnline } = useConnectivityStore.getState()
        setOnline(navigator.onLine)

        const handleOnline = () => setOnline(true)
        const handleOffline = () => setOnline(false)

        window.addEventListener('online', handleOnline)
        window.addEventListener('offline', handleOffline)

        return () => {
            window.removeEventListener('online', handleOnline)
            window.removeEventListener('offline', handleOffline)
        }
    }, [])
}
