import { useConnectivityStore } from '@tinycld/core/lib/stores/connectivity-store'
import { type WritesAvailability, writesAvailability } from '@tinycld/core/lib/writes-available'

/**
 * Whether a change can be saved right now, and why not. The shared submit
 * buttons read it (Dialog.ActionButton with `requiresServer`, ConfirmDialog);
 * a screen with its own submit button can read it the same way.
 */
export function useWritesAvailable(): WritesAvailability {
    const isOnline = useConnectivityStore(s => s.isOnline)
    const isServerReachable = useConnectivityStore(s => s.isServerReachable)
    return writesAvailability({ isOnline, isServerReachable })
}
