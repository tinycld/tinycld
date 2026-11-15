import { LoadingState } from '@tinycld/core/components/LoadingState'
import { DETAIL_PANEL_WIDTH } from '@tinycld/core/lib/detail-surface'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { LayerEscape } from '@tinycld/core/ui/overlay'
import { useLocalSearchParams } from 'expo-router'
import { Maximize2, X } from 'lucide-react-native'
import { Pressable, Text, View } from 'react-native'
import { type DrillPanelInput, type DrillPanelModel, useDrillPanel } from '../hooks/useDrillPanel'
import type { ReportViewModel } from '../hooks/useReportView'
import { useTableLayout } from '../hooks/useTableLayout'
import { firstOf } from '../lib/drill'
import { layoutNoticeOf } from '../lib/notices'
import { DRILL_KEY } from '../lib/view-url'
import { ReportControls } from './ReportControls'
import { ReportDocumentHeader } from './ReportDocumentHeader'
import { REPORT_TAP_TARGET, ReportBackLink } from './ReportHeader'
import { ReportNotices } from './ReportNotices'
import { MessageBody } from './ReportStates'
import { ReportBody } from './ReportView'

// Beside the report under a left border, or over it as a page: style values only,
// so the panel's report stays mounted when the window crosses 1360 px.
const PANEL_STYLE = { width: DETAIL_PANEL_WIDTH, borderLeftWidth: 1 } as const
const PAGE_STYLE = {
    position: 'absolute',
    top: 0,
    right: 0,
    bottom: 0,
    left: 0,
    borderLeftWidth: 0,
} as const
const TAP_STYLE = { minHeight: REPORT_TAP_TARGET, minWidth: REPORT_TAP_TARGET }

function PageBack({ panel }: { panel: DrillPanelModel }) {
    if (!panel.isPage) return null
    return <ReportBackLink label={panel.backLabel} onBack={panel.onClose} />
}

function CloseButton({ panel }: { panel: DrillPanelModel }) {
    const muted = useThemeColor('muted-foreground')
    if (panel.isPage) return null
    return (
        <Pressable
            testID="report-drill-close"
            accessibilityRole="button"
            accessibilityLabel="Close"
            onPress={panel.onClose}
            className="items-center justify-center"
            style={TAP_STYLE}
        >
            <X size={18} color={muted} />
        </Pressable>
    )
}

function OpenFullButton({ panel }: { panel: DrillPanelModel }) {
    const color = useThemeColor('foreground')
    if (panel.recordState !== 'ready') return null
    return (
        <Pressable
            testID="report-drill-open-full"
            accessibilityRole="button"
            onPress={panel.onOpenFull}
            className="flex-row items-center gap-1.5 px-2"
            style={{ minHeight: REPORT_TAP_TARGET }}
        >
            <Maximize2 size={16} color={color} />
            <Text className="text-sm text-foreground">Open as full report</Text>
        </Pressable>
    )
}

function DrillPanelHeader({ panel }: { panel: DrillPanelModel }) {
    return (
        <View testID="report-drill-panel-header" className="gap-1 border-b border-border px-4 py-2">
            <PageBack panel={panel} />
            <View className="flex-row items-center gap-2">
                <View
                    key={panel.titleKey}
                    ref={panel.titleRef}
                    testID="report-drill-panel-title"
                    tabIndex={-1}
                    accessibilityRole="header"
                    className="min-w-0 flex-1"
                >
                    <Text numberOfLines={1} className="text-lg font-semibold text-foreground">
                        {panel.title}
                    </Text>
                </View>
                <OpenFullButton panel={panel} />
                <CloseButton panel={panel} />
            </View>
        </View>
    )
}

function RecordLoading({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return (
        <View testID="report-drill-loading" className="flex-1">
            <LoadingState />
        </View>
    )
}

// The target's record: its own states before its report can run.
function DrillRecordStates({ panel }: { panel: DrillPanelModel }) {
    const retry = { label: 'Retry', testID: 'report-drill-retry', onPress: panel.retryRecord }
    return (
        <>
            <RecordLoading isVisible={panel.recordState === 'loading'} />
            <MessageBody
                isVisible={panel.recordState === 'error'}
                testID="report-drill-error"
                message="Could not load this report"
                action={retry}
            />
            <MessageBody
                isVisible={panel.recordState === 'missing'}
                testID="report-drill-no-access"
                message="You cannot open this report"
            />
        </>
    )
}

// The target's chips, notices, document header and table, fitted to the panel's
// width. No toolbar: Customize, Share, Save as, Copy link and Edit report are on
// the full report, one press away. Its rows open their records; it has no panel.
function DrillReport({ view, targetId }: { view: ReportViewModel; targetId: string }) {
    const table = useTableLayout(view)
    const layoutNotice = view.state === 'ready' ? layoutNoticeOf(table.layout) : undefined
    return (
        <View className="min-h-0 flex-1">
            <ReportControls model={view} isSortMenuShown={table.layout.kind === 'blocks'} />
            <ReportNotices
                notices={view.notices}
                layoutNotice={layoutNotice}
                isCompact={view.breakpoint === 'mobile'}
                width={view.viewWidth}
            />
            <ReportDocumentHeader
                heading={view.heading}
                title={view.title}
                periodWords={view.periodWords}
                isThousands={view.display.divideBy1000}
            />
            <ReportBody
                model={view}
                table={table}
                drill={{ sourceId: targetId, hasPanel: false, scope: 'drill' }}
            />
        </View>
    )
}

function DrillPanelBody({ panel }: { panel: DrillPanelModel }) {
    if (panel.recordState !== 'ready') return null
    return <DrillReport view={panel.view} targetId={panel.targetId} />
}

function DrillPanel(props: DrillPanelInput) {
    const panel = useDrillPanel(props)
    return (
        <View
            testID="report-drill-panel"
            role="complementary"
            aria-label={panel.title || 'Report'}
            className="border-border bg-background"
            style={panel.isPage ? PAGE_STYLE : PANEL_STYLE}
        >
            <LayerEscape onEscape={panel.onClose} />
            <DrillPanelHeader panel={panel} />
            <DrillRecordStates panel={panel} />
            <DrillPanelBody panel={panel} />
        </View>
    )
}

/**
 * The report a figure drilled to (the route's `d`), in ReportView's side slot. It
 * draws nothing without `d`, so Escape is registered only while it is open.
 */
export function ReportDrillPanel({
    sourceId,
    sourceTitle,
}: {
    sourceId: string
    sourceTitle: string
}) {
    const targetId = firstOf(useLocalSearchParams()[DRILL_KEY])
    if (!targetId) return null
    return <DrillPanel targetId={targetId} sourceId={sourceId} sourceTitle={sourceTitle} />
}
