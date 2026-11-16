import { useQuery } from '@tanstack/react-query'
import { errorToString } from '@tinycld/core/lib/errors'
import { log } from '@tinycld/core/lib/logger'
import { useMutation } from '@tinycld/core/lib/mutations'
import { saveFile } from '@tinycld/core/lib/save-file'
import { useToastStore } from '@tinycld/core/lib/stores/toast-store'
import { useWritesAvailable } from '@tinycld/core/lib/use-writes-available'
import { todayIso } from '../lib/dates'
import {
    type ExportFormat,
    ExportRefusal,
    type ExportRequest,
    exportFileName,
    exportFormatEntry,
    exportOffered,
    exportRequestOf,
    fetchEmailStatus,
    fetchExport,
    isExportDisabled,
} from '../lib/export-client'
import type { DisplayOptions } from '../lib/format'
import type { RunBody } from '../lib/run-client'
import type { ParamDef } from '../lib/spec'
import type { ResolvedParam } from '../lib/types'
import { useReportUIStore } from '../stores/report-ui-store'
import type { ViewSource, ViewState } from './useReportView'

export interface ReportExportInput {
    source: ViewSource | null
    /** The body the view runs; null when it runs nothing (a broken report). */
    body: RunBody | null
    display: DisplayOptions
    state: ViewState
    /** A changed run loads while the view still shows the last result. */
    isUpdating: boolean
    isEditing: boolean
    /** False for the drill panel, which has no Export. */
    isMainView: boolean
    /** Only the focused screen draws the email dialog. */
    isFocused: boolean
    title: string
    defs: readonly ParamDef[]
    /** The run's resolved params, which name the file's date part. */
    resolved: readonly ResolvedParam[]
}

export interface ReportExportModel {
    isDisabled: boolean
    isPreparing: boolean
    onDownload(format: ExportFormat): void
    request: ExportRequest | null
    title: string
    fileNameOf(format: ExportFormat): string
    email: ReportEmailModel
}

export interface ReportEmailModel {
    /** undefined until the server answers; true when the read failed (the send refuses then). */
    canSend: boolean | undefined
    isOpen: boolean
    onOpen(): void
    onClose(): void
}

const EMAIL_STATUS_STALE_MS = 5 * 60 * 1000

/** "Email report…" waits for the status; a failed read leaves it to the send's own refusal. */
function canSendOf(status: { data?: { canSend: boolean }; isError: boolean }) {
    if (status.isError) return true
    return status.data?.canSend
}

interface DownloadVariables {
    request: ExportRequest
    format: ExportFormat
    fallbackName: string
}

const FAILURE_TITLE = 'Could not create the file'

// A refusal the reader can cause (a row limit, a lost access) is expected; any
// other failure is a defect worth an error report.
function logFailure(error: unknown) {
    if (error instanceof ExportRefusal && error.status >= 400 && error.status < 500) {
        log.info('reports.export', 'export refused', {
            status: error.status,
            message: error.message,
        })
        return
    }
    log.error('reports.export', error)
}

function showFailure(error: unknown) {
    logFailure(error)
    useToastStore.getState().addToast({
        title: FAILURE_TITLE,
        body: errorToString(error),
        variant: 'error',
        duration: 6000,
    })
}

async function download({ request, format, fallbackName }: DownloadVariables) {
    const { bytes, fileName } = await fetchExport(request, format, fallbackName)
    const { mimeType, uti } = exportFormatEntry(format)
    await saveFile({ bytes, fileName, mimeType, uti })
}

function savedBodyOf(body: RunBody | null) {
    return body && 'reportId' in body ? body : null
}

/**
 * The Export button's model: null where Export is not offered (a draft, the
 * builder, the drill panel). It reads no access field: every reader the view
 * opens for may export, and the file holds what that reader's run returns. The
 * download never touches the run query, so the report on screen does not rerun.
 */
export function useReportExport(input: ReportExportInput): ReportExportModel | null {
    const writes = useWritesAvailable()
    const mutation = useMutation<void, Error, DownloadVariables>({
        mutationKey: ['reports', 'export'],
        // Offline is a failure to show now, not a request to hold.
        networkMode: 'always',
        mutationFn: download,
        onError: showFailure,
    })
    const isOffered = exportOffered({
        isSaved: input.source?.kind === 'saved',
        isEditing: input.isEditing,
        isMainView: input.isMainView,
    })
    const emailStatus = useQuery({
        queryKey: ['reports', 'email-status'],
        queryFn: fetchEmailStatus,
        staleTime: EMAIL_STATUS_STALE_MS,
        retry: false,
        enabled: isOffered,
    })
    const emailReportId = useReportUIStore(s => s.emailReportId)
    const openEmail = useReportUIStore(s => s.openEmail)
    const closeEmail = useReportUIStore(s => s.closeEmail)
    if (!isOffered || input.source?.kind !== 'saved') return null
    const reportId = input.source.reportId
    const saved = savedBodyOf(input.body)
    const request = saved ? exportRequestOf(saved, input.display) : null
    // The day the server names the file by: the run's day, the reader's own.
    const today = saved?.options.today ?? todayIso()
    const fileNameOf = (format: ExportFormat) =>
        exportFileName({
            title: input.title,
            defs: input.defs,
            resolved: input.resolved,
            today,
            format,
        })
    const isPreparing = mutation.isPending
    return {
        isDisabled: isExportDisabled({
            isPreparing,
            writesAvailable: writes.available,
            state: input.state,
            hasRequest: request !== null,
            isUpdating: input.isUpdating,
        }),
        isPreparing,
        onDownload: (format: ExportFormat) => {
            if (!request) return
            mutation.mutate({ request, format, fallbackName: fileNameOf(format) })
        },
        request,
        title: input.title,
        fileNameOf,
        email: {
            canSend: canSendOf(emailStatus),
            isOpen: emailReportId === reportId && input.isFocused,
            onOpen: () => openEmail(reportId),
            onClose: closeEmail,
        },
    }
}
