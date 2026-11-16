import { errorToString } from '@tinycld/core/lib/errors'
import { log } from '@tinycld/core/lib/logger'
import { useMutation } from '@tinycld/core/lib/mutations'
import { saveFile } from '@tinycld/core/lib/save-file'
import { useToastStore } from '@tinycld/core/lib/stores/toast-store'
import { useWritesAvailable } from '@tinycld/core/lib/use-writes-available'
import { todayIso } from '../lib/dates'
import {
    canExport,
    type ExportFormat,
    ExportRefusal,
    type ExportRequest,
    exportFileName,
    exportFormatEntry,
    exportOffered,
    exportRequestOf,
    fetchExport,
} from '../lib/export-client'
import type { DisplayOptions } from '../lib/format'
import type { RunBody } from '../lib/run-client'
import type { ParamDef } from '../lib/spec'
import type { ResolvedParam } from '../lib/types'
import type { ViewSource, ViewState } from './useReportView'

export interface ReportExportInput {
    source: ViewSource | null
    /** The body the view runs; null when it runs nothing (a broken report). */
    body: RunBody | null
    display: DisplayOptions
    state: ViewState
    isEditing: boolean
    /** False for the drill panel, which has no Export. */
    isMainView: boolean
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
    if (!isOffered) return null
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
        isDisabled: isPreparing || !writes.available || !canExport(input.state) || !request,
        isPreparing,
        onDownload: (format: ExportFormat) => {
            if (!request) return
            mutation.mutate({ request, format, fallbackName: fileNameOf(format) })
        },
        request,
        title: input.title,
        fileNameOf,
    }
}
