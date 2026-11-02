import { Text, View } from 'react-native'
import type { Notice } from '../lib/types'

// What the run says about itself: caps that applied, mixed currencies, an upgraded
// spec, renamed fields (spec "Errors": a notice above the table). "Updating…" is
// not one of them: it has its own slot in the header (ReportUpdating).
export function ReportNotices({ notices }: { notices: Notice[] }) {
    if (notices.length === 0) return null
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
        </View>
    )
}
