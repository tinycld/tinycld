import {
    type ConnectionIndicatorState,
    noticeLabel,
    serverHostLabel,
} from '@tinycld/core/lib/connection-indicator'
import { getResolvedAddress } from '@tinycld/core/lib/server-address'
import { useConnectionOptionsStore } from '@tinycld/core/lib/stores/connection-options-store'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useConnectionIndicator } from '@tinycld/core/lib/use-connection-indicator'
import { useConnectionOptions } from '@tinycld/core/lib/use-connection-options'
import { useServerRecoveryProbe } from '@tinycld/core/lib/use-server-recovery-probe'
import { Dialog } from '@tinycld/core/ui/dialog'
import { useInertExempt } from '@tinycld/core/ui/overlay/use-inert-exempt'
import { RefreshCw, ServerOff, WifiOff } from 'lucide-react-native'
import { Platform, Pressable, Text, View } from 'react-native'

const ICONS = { offline: WifiOff, reconnecting: RefreshCw, unreachable: ServerOff }

// On web the server is the page's own origin, so its host says nothing.
function displayedHost(): string | null {
    return Platform.OS === 'web' ? null : serverHostLabel(getResolvedAddress())
}

/**
 * The connection notice at the top of the app shell. Non-blocking: it never
 * covers the app. Short outages show a plain pill; once the server has not
 * answered for long enough (see connectionNotice) the pill becomes a button
 * that opens the connection options.
 */
export function ConnectionIndicator() {
    useServerRecoveryProbe()
    const state = useConnectionIndicator()
    return (
        <>
            <ConnectionPill state={state} />
            <ConnectionOptionsDialog />
        </>
    )
}

function ConnectionPill({ state }: { state: ConnectionIndicatorState }) {
    const iconColor = useThemeColor('warning')
    const openOptions = useConnectionOptionsStore(s => s.open)
    // An open dialog makes the app inert, which would also hide the notice
    // from screen readers and block the tap that opens the options.
    const inertExemptRef = useInertExempt()
    if (state === 'hidden') return null
    const Icon = ICONS[state]
    const label = noticeLabel(state, displayedHost())
    const isEscalated = state === 'unreachable'

    return (
        <View
            ref={inertExemptRef}
            pointerEvents="box-none"
            className="absolute top-2 left-0 right-0 items-center z-50"
            testID="connection-indicator"
        >
            <Pressable
                disabled={!isEscalated}
                onPress={openOptions}
                accessibilityRole={isEscalated ? 'button' : 'alert'}
                accessibilityLiveRegion="polite"
                accessibilityLabel={label}
                className="flex-row items-center gap-2 rounded-full border border-border bg-background px-3 py-1.5 shadow-sm"
            >
                <Icon size={14} color={iconColor} />
                <Text className="text-xs font-medium text-foreground">{label}</Text>
            </Pressable>
        </View>
    )
}

function ConnectionOptionsDialog() {
    const options = useConnectionOptions()
    const retryLabel = options.isRetrying ? 'Checking…' : 'Retry'

    return (
        <Dialog
            isOpen={options.isOpen}
            onClose={options.close}
            title="Can't reach the server"
            testID="connection-options"
        >
            <Dialog.Body>
                <Text className="text-sm text-muted-foreground">Server</Text>
                <Text className="text-sm text-foreground" selectable>
                    {options.address ?? 'No server chosen'}
                </Text>
                <RetryFailedMessage isVisible={options.retryFailed && !options.isRetrying} />
            </Dialog.Body>
            <Dialog.Footer>
                <ChooseServerButton
                    isVisible={options.canChooseServer}
                    onPress={options.chooseServer}
                />
                <Dialog.ActionButton
                    label={retryLabel}
                    onPress={options.retry}
                    isDisabled={options.isRetrying}
                    requiresServer={false}
                    testID="connection-options-retry"
                />
            </Dialog.Footer>
        </Dialog>
    )
}

function RetryFailedMessage({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return (
        <Text accessibilityLiveRegion="polite" className="mt-3 text-sm text-danger">
            Still can't reach the server. The app keeps trying on its own.
        </Text>
    )
}

function ChooseServerButton({ isVisible, onPress }: { isVisible: boolean; onPress: () => void }) {
    if (!isVisible) return null
    return <Dialog.CancelButton label="Choose another server" onPress={onPress} />
}
