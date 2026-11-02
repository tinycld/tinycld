import { Text, View } from 'react-native'
import { useIOSAnnouncement } from '../hooks/useIOSAnnouncement'

const UPDATING = 'Updating…'

function UpdatingText({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return (
        <Text testID="report-updating" numberOfLines={1} className="text-xs text-muted-foreground">
            {UPDATING}
        </Text>
    )
}

const SLOT_CLASS = 'h-6 w-20 shrink-0 items-end justify-center'

// "Updating…" shows while a changed run loads behind the figures in view. Its
// slot has a fixed size and is always there, in a header row, so nothing moves
// or is covered when the text comes and goes; the slot is a live region, so a
// screen reader announces the text when it appears (on iOS, an announcement).
// A slot that is not live (another surface over it shows the text) stays empty
// and silent, so the text is announced once.
export function ReportUpdating({
    isUpdating,
    isLive = true,
}: {
    isUpdating: boolean
    isLive?: boolean
}) {
    useIOSAnnouncement(UPDATING, isUpdating && isLive)
    if (!isLive) return <View testID="report-updating-slot" className={SLOT_CLASS} />
    return (
        <View
            testID="report-updating-slot"
            role="status"
            aria-live="polite"
            accessibilityLiveRegion="polite"
            className={SLOT_CLASS}
        >
            <UpdatingText isVisible={isUpdating} />
        </View>
    )
}
