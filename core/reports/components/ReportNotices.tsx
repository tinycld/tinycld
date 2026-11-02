import { Text, View } from 'react-native'
import type { Notice } from '../lib/types'

// "Updating…" shows while a changed run loads behind the figures in view. It
// floats over the top of what follows and takes no height, so the rows in view
// do not move down when a run starts loading or back up when it lands.
function Updating({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return (
        <View className="relative z-10 h-0" pointerEvents="none">
            <View className="absolute right-4 top-2 rounded-full border border-border bg-surface-secondary px-3 py-1">
                <Text
                    testID="report-updating"
                    accessibilityLiveRegion="polite"
                    className="text-xs text-muted-foreground"
                >
                    Updating…
                </Text>
            </View>
        </View>
    )
}

// What the run says about itself: caps that applied, mixed currencies, an upgraded
// spec, renamed fields (spec "Errors": a notice above the table).
function NoticeLines({ notices }: { notices: Notice[] }) {
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

interface ReportNoticesProps {
    notices: Notice[]
    isUpdating: boolean
}

export function ReportNotices({ notices, isUpdating }: ReportNoticesProps) {
    return (
        <>
            <NoticeLines notices={notices} />
            <Updating isVisible={isUpdating} />
        </>
    )
}
