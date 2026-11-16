import { Text, View } from 'react-native'
import { useIOSAnnouncement } from '../hooks/useIOSAnnouncement'

const UPDATING = 'Updating…'

function SlotText({ text, testID }: { text: string | null; testID: string }) {
    if (text === null) return null
    return (
        <Text testID={testID} numberOfLines={1} className="text-xs text-muted-foreground">
            {text}
        </Text>
    )
}

// What the slot shows: the reader's own work in progress (a file being prepared),
// else "Updating…" while a changed run loads, else nothing.
function slotTextOf(isUpdating: boolean, label: string | undefined) {
    if (label !== undefined) return { text: label, testID: 'report-preparing' }
    return { text: isUpdating ? UPDATING : null, testID: 'report-updating' }
}

const SLOT_CLASS = 'h-6 w-20 shrink-0 items-end justify-center'

// "Updating…" shows while a changed run loads behind the figures in view; `label`
// ("Preparing…" while a file is made) shows in its place. The slot has a fixed
// size and is always there, in a header row, so nothing moves or is covered when
// the text comes and goes; the slot is a live region, so a screen reader announces
// the text when it appears (on iOS, an announcement). A slot that is not live
// (another surface over it shows the text) stays empty and silent, so the text is
// announced once.
export function ReportUpdating({
    isUpdating,
    isLive = true,
    label,
}: {
    isUpdating: boolean
    isLive?: boolean
    label?: string
}) {
    const { text, testID } = slotTextOf(isUpdating, label)
    useIOSAnnouncement(text ?? '', text !== null && isLive)
    if (!isLive) return <View testID="report-updating-slot" className={SLOT_CLASS} />
    return (
        <View
            testID="report-updating-slot"
            role="status"
            aria-live="polite"
            accessibilityLiveRegion="polite"
            className={SLOT_CLASS}
        >
            <SlotText text={text} testID={testID} />
        </View>
    )
}
