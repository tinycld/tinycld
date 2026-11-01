import { Text, View } from 'react-native'
import type { Notice } from '../lib/types'

function Updating({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return (
        <Text testID="report-updating" className="text-xs text-muted-foreground">
            Updating…
        </Text>
    )
}

interface ReportNoticesProps {
    notices: Notice[]
    isUpdating: boolean
}

// What the run says about itself: caps that applied, mixed currencies, an upgraded
// spec, renamed fields (spec "Errors": a notice above the table). "Updating…" shows
// while a changed run loads behind the figures in view.
export function ReportNotices({ notices, isUpdating }: ReportNoticesProps) {
    if (notices.length === 0 && !isUpdating) return null
    // mergePages keeps one notice per code, so the code is a stable key.
    const lines = notices.map(notice => (
        <Text
            key={notice.code}
            testID={`report-notice-${notice.code}`}
            className="text-sm text-foreground"
        >
            {notice.message}
        </Text>
    ))
    return (
        <View
            testID="report-notices"
            accessibilityLiveRegion="polite"
            className="gap-1 border-t border-border bg-surface-secondary px-4 py-2"
        >
            {lines}
            <Updating isVisible={isUpdating} />
        </View>
    )
}
