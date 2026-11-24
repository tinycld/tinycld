import { PromptDialog } from '@tinycld/core/ui/PromptDialog'
import { router, useLocalSearchParams } from 'expo-router'
import { useState } from 'react'
import { View } from 'react-native'
import { ReportBuilderPanel } from '../components/builder/ReportBuilderPanel'
import { ReportDrillPanel } from '../components/ReportDrillPanel'
import { ReportHeader } from '../components/ReportHeader'
import { LoadingBody, MessageBody } from '../components/ReportStates'
import { ReportView } from '../components/ReportView'
import { ReportShareDialog } from '../components/share/ReportShareDialog'
import { useBackToReports, useReportBack } from '../hooks/useBackToReports'
import { useIsDrillPage } from '../hooks/useDrillPanel'
import { useHubActions } from '../hooks/useHubActions'
import { copyTitle, MAX_TITLE, useReportBuilder } from '../hooks/useReportBuilder'
import { useReportCatalog } from '../hooks/useReportCatalog'
import { useReportRecord } from '../hooks/useReportRecord'
import { useReportTermsGate } from '../hooks/useReportTermsGate'
import { useReportView } from '../hooks/useReportView'
import type { Catalog } from '../lib/builder/columns'
import { HUB_BACK_LABEL } from '../lib/report-back'
import { REPORTS_BASE, ReportBaseProvider } from '../lib/report-base'
import { canCopyReport, UNREADABLE_SPEC } from '../lib/spec'
import type { CatalogSource } from '../lib/types'
import { EDIT_KEY, FROM_KEY } from '../lib/view-url'
import { useReportUIStore } from '../stores/report-ui-store'

type RecordModel = ReturnType<typeof useReportRecord>

const NOT_INSTALLED = 'This kind of report is not installed'

interface FrameProps {
    title: string
    state: 'loading' | 'error' | 'no-access' | 'unknown-source'
    onBack: () => void
    backLabel?: string
    onRetry?: () => void
}

/** The frame shown while the catalog is not ready. */
export function catalogFrameState(catalog: { isError: boolean }): FrameProps['state'] {
    return catalog.isError ? 'error' : 'loading'
}

/** The report screen before its record or catalog is ready, or when it cannot be shown. */
export function ReportFrame({ title, state, onBack, backLabel, onRetry }: FrameProps) {
    const retry = onRetry ? { label: 'Retry', testID: 'report-retry', onPress: onRetry } : null
    return (
        <View testID="report-screen" className="flex-1 bg-background">
            <ReportHeader title={title} onBack={onBack} backLabel={backLabel} actions={[]} />
            <LoadingBody isVisible={state === 'loading'} />
            <MessageBody
                isVisible={state === 'error'}
                testID="report-error"
                message="Could not load this report"
                action={retry}
            />
            <MessageBody
                isVisible={state === 'no-access'}
                testID="report-no-access"
                message="You no longer have access to this report"
            />
            <MessageBody
                isVisible={state === 'unknown-source'}
                testID="report-unknown-source"
                message={NOT_INSTALLED}
            />
        </View>
    )
}

// The builder designs only a report it can read over an installed source, so Edit
// report is offered only then; a report whose source is gone says so instead.
function useDesignability(record: RecordModel) {
    const catalog = useReportCatalog()
    const source = record.spec?.source
    const isInstalled = source !== undefined && catalog.byKey.has(source)
    return {
        canDesign: isInstalled,
        missingNote: catalog.isReady && source !== undefined && !isInstalled ? NOT_INSTALLED : null,
    }
}

interface ReportBack {
    label: string
    onBack: () => void
}

function SavedReportView({ record, back }: { record: RecordModel; back: ReportBack }) {
    const report = record.record
    const openShare = useReportUIStore(s => s.openShare)
    const actions = useHubActions()
    const { canDesign, missingNote } = useDesignability(record)
    const isCovered = useIsDrillPage()
    // The Save as prompt is a modal toggle: local UI state.
    const [isSavingAs, setSavingAs] = useState(false)
    const canCopy = report !== undefined && canCopyReport(report)
    const model = useReportView({
        reportKey: report?.id ?? '',
        title: report?.title ?? '',
        defs: record.spec?.params ?? [],
        source: report && record.spec ? { kind: 'saved', reportId: report.id } : null,
        access: record.access,
        broken: record.broken ?? (record.spec ? null : UNREADABLE_SPEC),
        isEditing: false,
        onBack: back.onBack,
        backLabel: back.label,
        onEdit: canDesign ? () => router.setParams({ [EDIT_KEY]: '1' }) : undefined,
        missingNote,
        onShare: report ? () => openShare(report.id) : undefined,
        onSaveAs: canCopy ? () => setSavingAs(true) : undefined,
        spec: record.spec,
    })
    const onSaveCopy = (title: string) => {
        setSavingAs(false)
        if (report) actions.saveCopy(report, title)
    }
    // The figure opens its report beside this one on a wide window (D4), else as a screen.
    const drill = report
        ? { sourceId: report.id, hasPanel: true, scope: 'main' as const }
        : undefined
    const side = report ? (
        <ReportDrillPanel sourceId={report.id} sourceTitle={report.title} />
    ) : null
    return (
        <>
            <ReportView model={model} drill={drill} side={side} isCovered={isCovered} />
            <ReportShareDialog />
            <PromptDialog
                isOpen={isSavingAs}
                onClose={() => setSavingAs(false)}
                onSubmit={onSaveCopy}
                title="Save as"
                placeholder="Report title"
                confirmLabel="Save copy"
                defaultValue={copyTitle(report?.title ?? '')}
                maxLength={MAX_TITLE}
                required
            />
        </>
    )
}

function EditingBody({ record, back }: { record: RecordModel; back: ReportBack }) {
    const catalog = useReportCatalog()
    const report = record.record
    const spec = record.spec
    const base = spec ? catalog.byKey.get(spec.source) : undefined
    if (!catalog.isReady) {
        return (
            <ReportFrame
                title={report?.title ?? ''}
                state={catalogFrameState(catalog)}
                onBack={back.onBack}
                backLabel={back.label}
                onRetry={catalog.retry}
            />
        )
    }
    // A report whose source is gone cannot be designed; its view names what is missing.
    if (!report || !spec || !base) return <SavedReportView record={record} back={back} />
    return <EditingReport record={record} catalog={catalog.byKey} base={base} back={back} />
}

interface EditingReportProps {
    record: RecordModel
    catalog: Catalog
    base: CatalogSource
    back: ReportBack
}

function EditingReport({ record, catalog, base, back }: EditingReportProps) {
    const builder = useReportBuilder({
        mode: 'edit',
        catalog,
        base,
        record: record.record ?? null,
        spec: record.spec,
    })
    const model = useReportView({
        reportKey: record.record?.id ?? '',
        title: builder.title || (record.record?.title ?? ''),
        defs: builder.previewSpec?.params ?? [],
        source: builder.previewSpec ? { kind: 'draft', spec: builder.previewSpec } : null,
        access: record.access,
        broken: null,
        isEditing: true,
        onBack: back.onBack,
        backLabel: back.label,
        spec: builder.previewSpec,
    })
    return (
        <ReportView model={model} side={<ReportBuilderPanel builder={builder} preview={model} />} />
    )
}

// A drilled report (`from`) names its source on its back link; the hub's reads
// Reports and reads no record.
interface ReportScreenProps {
    reportId: string
    /**
     * The route this screen is drawn under, without the org prefix, when it is not
     * core's own: a drill and a saved copy open `<basePath>/<id>`, and Back with no
     * history opens `<basePath>`. The host owns both routes. Edit stays on the
     * current route (`?edit=1`). New report and the copied share link are core's
     * routes.
     */
    basePath?: string
}

export function ReportScreen({ reportId, basePath = REPORTS_BASE }: ReportScreenProps) {
    return (
        <ReportBaseProvider base={basePath}>
            <ReportByOrigin reportId={reportId} />
        </ReportBaseProvider>
    )
}

function ReportByOrigin({ reportId }: { reportId: string }) {
    const search = useLocalSearchParams<{ [FROM_KEY]?: string }>()
    const fromId = search[FROM_KEY]
    if (fromId) return <ReportFromSource reportId={reportId} fromId={fromId} />
    return <ReportFromHub reportId={reportId} />
}

function ReportFromSource({ reportId, fromId }: { reportId: string; fromId: string }) {
    const back = useReportBack(fromId)
    return <ReportScreenBody reportId={reportId} back={back} />
}

function ReportFromHub({ reportId }: { reportId: string }) {
    const onBack = useBackToReports()
    return <ReportScreenBody reportId={reportId} back={{ label: HUB_BACK_LABEL, onBack }} />
}

function ReportScreenBody({ reportId, back }: { reportId: string; back: ReportBack }) {
    const record = useReportRecord(reportId)
    const terms = useReportTermsGate(record.record?.source)
    const search = useLocalSearchParams<{ [EDIT_KEY]?: string }>()
    const frame = { onBack: back.onBack, backLabel: back.label }
    if (record.state === 'loading' || terms === 'wait') {
        return <ReportFrame title="" state="loading" {...frame} />
    }
    if (record.state === 'error') {
        return <ReportFrame title="" state="error" onRetry={record.retry} {...frame} />
    }
    if (record.state === 'missing' || terms === 'closed') {
        return <ReportFrame title="" state="no-access" {...frame} />
    }
    if (search[EDIT_KEY] === '1' && record.access.canEdit) {
        return <EditingBody record={record} back={back} />
    }
    return <SavedReportView record={record} back={back} />
}
