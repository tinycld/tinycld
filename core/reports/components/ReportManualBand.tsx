import type { ScopeOwner } from '@tinycld/core/lib/shortcuts/scopes'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { ShortcutButton } from '@tinycld/core/ui/shortcut-button'
import { Text, View } from 'react-native'
import type { ReportRun } from '../hooks/useReportRun'

export const MANUAL_NOTE =
    'This report does not update by itself. Press Refresh to see new changes.'

const REFRESH_LABEL = 'Refresh'
const REFRESHING_LABEL = 'Refreshing…'
const REFRESH_SHORTCUT = { keys: 'r', scope: 'list', group: 'Report' } as const
const NO_REFRESH = () => {}

interface RefreshButtonProps {
    testID: string
    isRefreshing: boolean
    onPress: () => void
    // A run with no refresh function has nothing to run: the button stays off.
    isDisabled: boolean
}

// With a `scopeOwner` the button has the shortcut R. A second report on the same
// screen (the drill panel, the builder's preview) gives none, so one key never
// runs two reports. The label says a Refresh runs: not every surface that shows
// the band has an "Updating…" slot.
function RefreshButton({
    testID,
    isRefreshing,
    onPress,
    isDisabled,
    scopeOwner,
}: RefreshButtonProps & { scopeOwner?: ScopeOwner }) {
    if (scopeOwner) {
        return (
            <ShortcutButton
                testID={testID}
                label={isRefreshing ? REFRESHING_LABEL : REFRESH_LABEL}
                shortcut={REFRESH_SHORTCUT}
                variant="outline"
                isDisabled={isDisabled}
                onPress={onPress}
                scopeOwner={scopeOwner}
            />
        )
    }
    return (
        <Button testID={testID} variant="outline" isDisabled={isDisabled} onPress={onPress}>
            <ButtonText>{isRefreshing ? REFRESHING_LABEL : REFRESH_LABEL}</ButtonText>
        </Button>
    )
}

// The band of a manual run (a run over a source too large to follow): the note
// that says the figures do not update by themselves, and Refresh. It is the same
// on every width, so a phone has the button in view and not under "⋯".
export function ReportManualBand({
    run,
    scopeOwner,
    idPrefix = 'report',
}: {
    run: ReportRun
    /** The screen's `list` scope owner: Refresh has the shortcut R. Not given, it has none. */
    scopeOwner?: ScopeOwner
    /** Starts the test ids, so each surface's band is told from the report's. */
    idPrefix?: string
}) {
    if (!run.isManual) return null
    return (
        <View testID={`${idPrefix}-manual-band`} className="flex-row items-center gap-3 px-4 py-1">
            <Text
                testID={`${idPrefix}-manual-note`}
                className="min-w-0 flex-1 text-sm text-muted-foreground"
            >
                {MANUAL_NOTE}
            </Text>
            <RefreshButton
                testID={`${idPrefix}-refresh`}
                isRefreshing={run.isRefreshing === true}
                onPress={run.refresh ?? NO_REFRESH}
                isDisabled={run.isRefreshing === true || !run.refresh}
                scopeOwner={scopeOwner}
            />
        </View>
    )
}
