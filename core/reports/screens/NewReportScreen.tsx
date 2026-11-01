import { useLocalSearchParams } from 'expo-router'
import { ReportBuilderPanel } from '../components/builder/ReportBuilderPanel'
import { ReportView } from '../components/ReportView'
import { useBackToReports } from '../hooks/useBackToReports'
import { useReportBuilder } from '../hooks/useReportBuilder'
import { useReportCatalog } from '../hooks/useReportCatalog'
import { useReportView } from '../hooks/useReportView'
import type { ReportAccess } from '../lib/access'
import type { Catalog } from '../lib/builder/columns'
import type { CatalogSource } from '../lib/types'
import { NEW_REPORT_KEY } from '../stores/report-ui-store'
import { catalogFrameState, ReportFrame } from './ReportScreen'

// A draft the reader is making: theirs to design and save, not yet shareable.
const DRAFT_ACCESS: ReportAccess = {
    isOwner: true,
    canEdit: true,
    canShare: false,
    canPublish: false,
    canDelete: false,
    isShipped: false,
}

interface NewReportProps {
    catalog: Catalog
    base: CatalogSource
    onBack: () => void
}

function NewReport({ catalog, base, onBack }: NewReportProps) {
    const builder = useReportBuilder({ mode: 'new', catalog, base, record: null, spec: null })
    const model = useReportView({
        reportKey: NEW_REPORT_KEY,
        title: builder.title || 'New report',
        defs: builder.previewSpec?.params ?? [],
        source: builder.previewSpec ? { kind: 'draft', spec: builder.previewSpec } : null,
        access: DRAFT_ACCESS,
        broken: null,
        isEditing: true,
        onBack,
    })
    return (
        <ReportView model={model} side={<ReportBuilderPanel builder={builder} preview={model} />} />
    )
}

export function NewReportScreen() {
    const { source } = useLocalSearchParams<{ source?: string }>()
    const catalog = useReportCatalog()
    const onBack = useBackToReports()
    const base = catalog.byKey.get(source ?? '')
    if (!catalog.isReady) {
        return (
            <ReportFrame
                title="New report"
                state={catalogFrameState(catalog)}
                onBack={onBack}
                onRetry={catalog.retry}
            />
        )
    }
    if (!base) return <ReportFrame title="New report" state="unknown-source" onBack={onBack} />
    return <NewReport catalog={catalog.byKey} base={base} onBack={onBack} />
}
