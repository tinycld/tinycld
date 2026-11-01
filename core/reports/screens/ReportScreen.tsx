import { PromptDialog } from '@tinycld/core/ui/PromptDialog'
import { router, useLocalSearchParams } from 'expo-router'
import { useState } from 'react'
import { View } from 'react-native'
import { ReportBuilderPanel } from '../components/builder/ReportBuilderPanel'
import { ReportHeader } from '../components/ReportHeader'
import { LoadingBody, MessageBody } from '../components/ReportStates'
import { ReportView } from '../components/ReportView'
import { ReportShareDialog } from '../components/share/ReportShareDialog'
import { useBackToReports } from '../hooks/useBackToReports'
import { useHubActions } from '../hooks/useHubActions'
import { copyTitle, MAX_TITLE, useReportBuilder } from '../hooks/useReportBuilder'
import { useReportCatalog } from '../hooks/useReportCatalog'
import { useReportRecord } from '../hooks/useReportRecord'
import { useReportView } from '../hooks/useReportView'
import type { Catalog } from '../lib/builder/columns'
import { type BrokenMark, canCopyReport } from '../lib/spec'
import type { CatalogSource } from '../lib/types'
import { EDIT_KEY } from '../lib/view-url'
import { useReportUIStore } from '../stores/report-ui-store'

type RecordModel = ReturnType<typeof useReportRecord>

// A stored spec this app cannot read (a newer spec version) cannot run here.
const UNREADABLE: BrokenMark = {
    status: 400,
    problems: {
        spec: {
            code: 'unreadable',
            message: 'This report was saved by a newer version of the app.',
        },
    },
}

const NOT_INSTALLED = 'This kind of report is not installed'

interface FrameProps {
    title: string
    state: 'loading' | 'error' | 'no-access' | 'unknown-source'
    onBack: () => void
    onRetry?: () => void
}

/** The frame shown while the catalog is not ready. */
export function catalogFrameState(catalog: { isError: boolean }): FrameProps['state'] {
    return catalog.isError ? 'error' : 'loading'
}

/** The report screen before its record or catalog is ready, or when it cannot be shown. */
export function ReportFrame({ title, state, onBack, onRetry }: FrameProps) {
    const retry = onRetry ? { label: 'Retry', testID: 'report-retry', onPress: onRetry } : null
    return (
        <View testID="report-screen" className="flex-1 bg-background">
            <ReportHeader title={title} onBack={onBack} toolbar={null} />
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

function SavedReportView({ record, onBack }: { record: RecordModel; onBack: () => void }) {
    const report = record.record
    const openShare = useReportUIStore(s => s.openShare)
    const actions = useHubActions()
    const { canDesign, missingNote } = useDesignability(record)
    // The Save as prompt is a modal toggle: local UI state.
    const [isSavingAs, setSavingAs] = useState(false)
    const canCopy = report !== undefined && canCopyReport(report)
    const model = useReportView({
        reportKey: report?.id ?? '',
        title: report?.title ?? '',
        defs: record.spec?.params ?? [],
        source: report && record.spec ? { kind: 'saved', reportId: report.id } : null,
        access: record.access,
        broken: record.broken ?? (record.spec ? null : UNREADABLE),
        isEditing: false,
        onBack,
        onEdit: canDesign ? () => router.setParams({ [EDIT_KEY]: '1' }) : undefined,
        missingNote,
        onShare: report ? () => openShare(report.id) : undefined,
        onSaveAs: canCopy ? () => setSavingAs(true) : undefined,
    })
    const onSaveCopy = (title: string) => {
        setSavingAs(false)
        if (report) actions.saveCopy(report, title)
    }
    return (
        <>
            <ReportView model={model} />
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

function EditingBody({ record, onBack }: { record: RecordModel; onBack: () => void }) {
    const catalog = useReportCatalog()
    const report = record.record
    const spec = record.spec
    const base = spec ? catalog.byKey.get(spec.source) : undefined
    if (!catalog.isReady) {
        return (
            <ReportFrame
                title={report?.title ?? ''}
                state={catalogFrameState(catalog)}
                onBack={onBack}
                onRetry={catalog.retry}
            />
        )
    }
    // A report whose source is gone cannot be designed; its view names what is missing.
    if (!report || !spec || !base) return <SavedReportView record={record} onBack={onBack} />
    return <EditingReport record={record} catalog={catalog.byKey} base={base} onBack={onBack} />
}

interface EditingReportProps {
    record: RecordModel
    catalog: Catalog
    base: CatalogSource
    onBack: () => void
}

function EditingReport({ record, catalog, base, onBack }: EditingReportProps) {
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
        onBack,
    })
    return (
        <ReportView model={model} side={<ReportBuilderPanel builder={builder} preview={model} />} />
    )
}

export function ReportScreen({ reportId }: { reportId: string }) {
    const record = useReportRecord(reportId)
    const search = useLocalSearchParams<{ [EDIT_KEY]?: string }>()
    const onBack = useBackToReports()
    if (record.state === 'loading') return <ReportFrame title="" state="loading" onBack={onBack} />
    if (record.state === 'error') {
        return <ReportFrame title="" state="error" onBack={onBack} onRetry={record.retry} />
    }
    if (record.state === 'missing')
        return <ReportFrame title="" state="no-access" onBack={onBack} />
    if (search[EDIT_KEY] === '1' && record.access.canEdit) {
        return <EditingBody record={record} onBack={onBack} />
    }
    return <SavedReportView record={record} onBack={onBack} />
}
