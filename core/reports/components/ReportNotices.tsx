import { Text, View } from 'react-native'
import type { Notice } from '../lib/types'

function NoticeBand({ notices }: { notices: Notice[] }) {
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
            className="gap-1 border-t border-border bg-surface-secondary px-4 py-2"
        >
            {lines}
        </View>
    )
}

// What the run says about itself: caps that applied, mixed currencies, an upgraded
// spec, renamed fields (spec "Errors": a notice above the table). "Updating…" is
// not one of them: it has its own slot in the header (ReportUpdating). The live
// region is always mounted, empty and without height when there are no notices,
// so a screen reader announces notices when they appear in it. Under the phone's
// builder sheet, whose Preview shows the notices again, the view's band is not a
// live region, so each notice is announced once.
export function ReportNotices({ notices, isLive = true }: { notices: Notice[]; isLive?: boolean }) {
    if (!isLive) return <NoticeBand notices={notices} />
    return (
        <View
            testID="report-notices-region"
            role="status"
            aria-live="polite"
            accessibilityLiveRegion="polite"
        >
            <NoticeBand notices={notices} />
        </View>
    )
}
