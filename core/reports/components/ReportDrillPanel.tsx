import { LoadingState } from '@tinycld/core/components/LoadingState'
import { DETAIL_PANEL_WIDTH } from '@tinycld/core/lib/detail-surface'
import { log } from '@tinycld/core/lib/logger'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { LayerEscape } from '@tinycld/core/ui/overlay'
import { useLocalSearchParams } from 'expo-router'
import { Maximize2, X } from 'lucide-react-native'
import { Component, type ReactNode, Suspense } from 'react'
import { Pressable, Text, View } from 'react-native'
import {
    type DrillPanelInput,
    type DrillPanelModel,
    type RecordPanelInput,
    type RecordPanelModel,
    useDrillPanel,
    useRecordPanel,
} from '../hooks/useDrillPanel'
import { PanelTitleProvider } from '../hooks/usePanelTitle'
import type { ReportViewModel } from '../hooks/useReportView'
import { useTableLayout } from '../hooks/useTableLayout'
import { firstOf } from '../lib/drill'
import { layoutNoticeOf } from '../lib/notices'
import { DRILL_KEY, RECORD_KEY } from '../lib/view-url'
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

interface PanelPageBackProps {
    isVisible: boolean
    label: string
    onBack: () => void
    testID?: string
}

function PanelPageBack({ isVisible, label, onBack, testID }: PanelPageBackProps) {
    if (!isVisible) return null
    return <ReportBackLink label={label} onBack={onBack} testID={testID} />
}

interface PanelCloseButtonProps {
    isVisible: boolean
    testID: string
    onClose: () => void
}

function PanelCloseButton({ isVisible, testID, onClose }: PanelCloseButtonProps) {
    const muted = useThemeColor('muted-foreground')
    if (!isVisible) return null
    return (
        <Pressable
            testID={testID}
            accessibilityRole="button"
            accessibilityLabel="Close"
            onPress={onClose}
            className="items-center justify-center"
            style={TAP_STYLE}
        >
            <X size={18} color={muted} />
        </Pressable>
    )
}

interface OpenFullButtonProps {
    isVisible: boolean
    testID: string
    label: string
    onPress: () => void
}

function OpenFullButton({ isVisible, testID, label, onPress }: OpenFullButtonProps) {
    const color = useThemeColor('foreground')
    if (!isVisible) return null
    return (
        <Pressable
            testID={testID}
            accessibilityRole="button"
            onPress={onPress}
            className="flex-row items-center gap-1.5 px-2"
            style={{ minHeight: REPORT_TAP_TARGET }}
        >
            <Maximize2 size={16} color={color} />
            <Text className="text-sm text-foreground">{label}</Text>
        </Pressable>
    )
}

interface PanelTitleProps {
    titleKey: number
    titleRef: DrillPanelModel['titleRef']
    title: string
    testID: string
}

// The focus lands on it when a press opens the panel; its key remounts it per press.
function PanelTitle({ titleKey, titleRef, title, testID }: PanelTitleProps) {
    return (
        <View
            key={titleKey}
            ref={titleRef}
            testID={testID}
            tabIndex={-1}
            accessibilityRole="header"
            className="min-w-0 flex-1"
        >
            <Text numberOfLines={1} className="text-lg font-semibold text-foreground">
                {title}
            </Text>
        </View>
    )
}

function DrillPanelHeader({ panel }: { panel: DrillPanelModel }) {
    return (
        <View testID="report-drill-panel-header" className="gap-1 border-b border-border px-4 py-2">
            <PanelPageBack
                isVisible={panel.isPage}
                label={panel.backLabel}
                onBack={panel.onClose}
            />
            <View className="flex-row items-center gap-2">
                <PanelTitle
                    titleKey={panel.titleKey}
                    titleRef={panel.titleRef}
                    title={panel.title}
                    testID="report-drill-panel-title"
                />
                <OpenFullButton
                    isVisible={panel.recordState === 'ready'}
                    testID="report-drill-open-full"
                    label="Open as full report"
                    onPress={panel.onOpenFull}
                />
                <PanelCloseButton
                    isVisible={!panel.isPage}
                    testID="report-drill-close"
                    onClose={panel.onClose}
                />
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
                zoneWords={view.zoneWords}
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
            aria-label={panel.ariaLabel}
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

function RecordPanelHeader({ panel }: { panel: RecordPanelModel }) {
    return (
        <View
            testID="report-record-panel-header"
            className="gap-1 border-b border-border px-4 py-2"
        >
            <PanelPageBack
                isVisible={panel.isPage}
                label={panel.backLabel}
                onBack={panel.onClose}
                testID="report-record-panel-back"
            />
            <View className="flex-row items-center gap-2">
                <PanelTitle
                    titleKey={panel.titleKey}
                    titleRef={panel.titleRef}
                    title={panel.title}
                    testID="report-record-panel-title"
                />
                <OpenFullButton
                    isVisible
                    testID="report-record-panel-full"
                    label="Open as full screen"
                    onPress={panel.onOpenFull}
                />
                <PanelCloseButton
                    isVisible={!panel.isPage}
                    testID="report-record-panel-close"
                    onClose={panel.onClose}
                />
            </View>
        </View>
    )
}

interface RecordBoundaryState {
    hasError: boolean
}

interface RecordBoundaryProps {
    children: ReactNode
    /** Drops the panel's failed chunk, so the next record loads it again (R14). */
    onError: () => void
    /** Changes with each row press; a press while the error shows tries the load again. */
    resetKey: number
}

// A package's panel that fails to load or throws: the panel says so and keeps its
// header (Open as full screen, ✕) and Escape; the report beside it stays usable.
// It is keyed by the record's path, so another record starts fresh; a new row press
// on the same record clears the error, so the reloaded chunk loads again (R14).
class RecordBoundary extends Component<RecordBoundaryProps, RecordBoundaryState> {
    state: RecordBoundaryState = { hasError: false }

    static getDerivedStateFromError(): RecordBoundaryState {
        return { hasError: true }
    }

    componentDidUpdate(prev: RecordBoundaryProps) {
        if (this.state.hasError && prev.resetKey !== this.props.resetKey) {
            this.setState({ hasError: false })
        }
    }

    // The boundary cannot tell a failed chunk from a throw while rendering, so every
    // error drops the chunk; for a chunk that loaded, the next load is a cheap
    // re-import. It runs after the error text replaced the panel, so the reload
    // starts no load for this record; the next row press clears the error and loads
    // the chunk again.
    componentDidCatch(error: Error) {
        log.error('reports.record-panel', error)
        this.props.onError()
    }

    render() {
        if (this.state.hasError) {
            return (
                <MessageBody
                    isVisible
                    testID="report-record-panel-error"
                    message="Could not load this record"
                />
            )
        }
        return this.props.children
    }
}

function RecordPanelLoading() {
    return (
        <View testID="report-record-panel-loading" className="flex-1">
            <LoadingState />
        </View>
    )
}

function RecordBody({ panel }: { panel: RecordPanelModel }) {
    const { Component: Panel, segments, open, reloadChunk } = panel
    return (
        <View className="min-h-0 flex-1">
            <PanelTitleProvider open={open}>
                <RecordBoundary key={open} onError={reloadChunk} resetKey={panel.titleKey}>
                    <Suspense fallback={<RecordPanelLoading />}>
                        <Panel segments={segments} />
                    </Suspense>
                </RecordBoundary>
            </PanelTitleProvider>
        </View>
    )
}

function RecordPanel({ panel }: { panel: RecordPanelModel }) {
    return (
        <View
            testID="report-record-panel"
            role="complementary"
            aria-label={panel.ariaLabel}
            className="border-border bg-background"
            style={panel.isPage ? PAGE_STYLE : PANEL_STYLE}
        >
            <LayerEscape onEscape={panel.onClose} />
            <RecordPanelHeader panel={panel} />
            <RecordBody panel={panel} />
        </View>
    )
}

function RecordPanelHost(props: RecordPanelInput) {
    const panel = useRecordPanel(props)
    if (!panel) return null
    return <RecordPanel panel={panel} />
}

/**
 * The report's side panel, in ReportView's side slot: the report a figure drilled
 * to (the route's `d`), or the record a row opened (`dr`, D5). One at a time; a
 * hand-edited URL that holds both shows the drill. It draws nothing without either,
 * so Escape is registered only while it is open.
 */
export function ReportDrillPanel({
    sourceId,
    sourceTitle,
}: {
    sourceId: string
    sourceTitle: string
}) {
    const search = useLocalSearchParams()
    const targetId = firstOf(search[DRILL_KEY])
    const open = firstOf(search[RECORD_KEY])
    if (targetId) {
        return <DrillPanel targetId={targetId} sourceId={sourceId} sourceTitle={sourceTitle} />
    }
    if (open) return <RecordPanelHost open={open} sourceTitle={sourceTitle} />
    return null
}
