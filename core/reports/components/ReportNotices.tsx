import { useWindowWidth } from '@tinycld/core/components/workspace/useBreakpoint'
import { useState } from 'react'
import { Pressable, Text, View } from 'react-native'
import { BAND_LINES, bandNotices, isBandCut } from '../lib/notices'
import type { Notice } from '../lib/types'
import { REPORT_TAP_TARGET } from './ReportHeader'

// px-4 on each side of the band.
const BAND_PADDING = 32
const TOGGLE_STYLE = { minHeight: REPORT_TAP_TARGET }

// On a phone a band of more than BAND_LINES lines is cut there until the reader
// asks for all of it (H5). The choice is this band's own UI state.
function useNoticeBand(notices: Notice[], isCompact: boolean) {
    const [isExpanded, setIsExpanded] = useState(false)
    const width = useWindowWidth() - BAND_PADDING
    const isCut =
        isCompact &&
        isBandCut(
            notices.map(notice => notice.message),
            width
        )
    return {
        isCut,
        isCapped: isCut && !isExpanded,
        isExpanded,
        toggleText: isExpanded ? 'Show less' : 'Show all',
        onToggle: () => setIsExpanded(open => !open),
    }
}

// mergePages keeps one notice per code, and the layout notice has its own code,
// so the code is a stable key.
function noticeLines(notices: Notice[]) {
    return notices.map(notice => (
        <Text
            key={notice.code}
            testID={`report-notice-${notice.code}`}
            className="text-sm text-foreground"
        >
            {notice.message}
        </Text>
    ))
}

// Capped: the notices run on as one paragraph, cut at BAND_LINES lines.
function noticeParagraph(notices: Notice[]) {
    const spans = notices.map(notice => (
        <Text key={notice.code} testID={`report-notice-${notice.code}`}>
            {`${notice.message} `}
        </Text>
    ))
    return (
        <Text numberOfLines={BAND_LINES} className="text-sm text-foreground">
            {spans}
        </Text>
    )
}

function BandToggle(props: {
    isVisible: boolean
    isExpanded: boolean
    text: string
    onPress(): void
}) {
    if (!props.isVisible) return null
    return (
        <Pressable
            testID="report-notices-toggle"
            accessibilityRole="button"
            accessibilityState={{ expanded: props.isExpanded }}
            onPress={props.onPress}
            className="justify-center self-start"
            style={TOGGLE_STYLE}
        >
            <Text className="text-sm font-semibold text-primary">{props.text}</Text>
        </Pressable>
    )
}

function NoticeBand({ notices, isCompact }: { notices: Notice[]; isCompact: boolean }) {
    const band = useNoticeBand(notices, isCompact)
    if (notices.length === 0) return null
    const body = band.isCapped ? noticeParagraph(notices) : noticeLines(notices)
    return (
        <View
            testID="report-notices"
            className="gap-1 border-t border-border bg-surface-secondary px-4 py-2"
        >
            {body}
            <BandToggle
                isVisible={band.isCut}
                isExpanded={band.isExpanded}
                text={band.toggleText}
                onPress={band.onToggle}
            />
        </View>
    )
}

// What the run says about itself (caps that applied, mixed currencies,
// renamed fields; spec "Errors"), and, last, the table's own layout notice
// (H5): one band above the table. "Updating…" is not one of them: it has its own
// slot in the header (ReportUpdating). The live region is always mounted, empty and
// without height when there are no notices, so a screen reader announces notices
// when they appear in it. Under the phone's builder sheet, whose Preview shows the
// notices again, the view's band is not a live region, so each notice is announced once.
export function ReportNotices({
    notices,
    layoutNotice,
    isLive = true,
    isCompact = false,
}: {
    notices: Notice[]
    layoutNotice?: string
    isLive?: boolean
    isCompact?: boolean
}) {
    const all = bandNotices(notices, layoutNotice)
    if (!isLive) return <NoticeBand notices={all} isCompact={isCompact} />
    return (
        <View
            testID="report-notices-region"
            role="status"
            aria-live="polite"
            accessibilityLiveRegion="polite"
        >
            <NoticeBand notices={all} isCompact={isCompact} />
        </View>
    )
}
