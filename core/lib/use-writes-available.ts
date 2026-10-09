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

/**
 * Same merge `ServerActionButton` does internally (disabled-or-unavailable,
 * hint-or-reason), for a server-write action whose visual can't be a plain
 * `Button` — an icon-only row action, a list-row pill, a toggle row. Compose
 * the result's `isDisabled`/`accessibilityHint` into the custom `Pressable`
 * instead of re-deriving the outage logic at each call site.
 */
export function useServerActionState(
    own: { isDisabled?: boolean; accessibilityHint?: string } = {}
): { isDisabled: boolean; accessibilityHint: string | undefined } {
    const writes = useWritesAvailable()
    return {
        isDisabled: !!own.isDisabled || !writes.available,
        accessibilityHint: writes.available ? own.accessibilityHint : writes.reason,
    }
}
