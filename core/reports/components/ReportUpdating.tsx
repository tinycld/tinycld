import { Text, View } from 'react-native'

function UpdatingText({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return (
        <Text testID="report-updating" numberOfLines={1} className="text-xs text-muted-foreground">
            Updating…
        </Text>
    )
}

// "Updating…" shows while a changed run loads behind the figures in view. Its
// slot has a fixed size and is always there, in a header row, so nothing moves
// or is covered when the text comes and goes; the slot is a live region, so a
// screen reader announces the text when it appears.
export function ReportUpdating({ isUpdating }: { isUpdating: boolean }) {
    return (
        <View
            testID="report-updating-slot"
            role="status"
            aria-live="polite"
            accessibilityLiveRegion="polite"
            className="h-6 w-20 shrink-0 items-end justify-center"
        >
            <UpdatingText isVisible={isUpdating} />
        </View>
    )
}
