import { isRecord } from '@tinycld/core/lib/errors'
import { pb } from '@tinycld/core/lib/pocketbase'
import { fileNameFromDisposition } from '@tinycld/core/lib/save-file'
import { serverFetch } from '@tinycld/core/lib/server-fetch'
import type { ViewState } from '../hooks/useReportView'
import { readerHour12, readerTimeZone } from './date-time'
import { isIsoDay } from './dates'
import type { DisplayOptions } from './format'
import { firstDateParam } from './params'
import type { RunBody, RunOptions } from './run-client'
import type { ParamDef } from './spec'
import type { ResolvedParam } from './types'

// The export route (core/server/reports/export.go): the whole report as a file,
// made on the server with the view's state. The client never makes a file.

export const EXPORT_PATH = '/api/reports/export'

export type ExportFormat = 'pdf' | 'xlsx' | 'csv'

export interface ExportFormatEntry {
    format: ExportFormat
    /** The format's name, as the email dialog's "Attach as" lists it. */
    label: string
    menuLabel: string
    mimeType: string
    /** The iOS Uniform Type Identifier the share sheet offers the file as. */
    uti: string
}

/** The formats in the order the Export menu lists them. */
export const EXPORT_FORMATS: readonly ExportFormatEntry[] = [
    {
        format: 'pdf',
        label: 'PDF',
        menuLabel: 'Download PDF',
        mimeType: 'application/pdf',
        uti: 'com.adobe.pdf',
    },
    {
        format: 'xlsx',
        label: 'Excel',
        menuLabel: 'Download Excel',
        mimeType: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
        uti: 'org.openxmlformats.spreadsheetml.sheet',
    },
    {
        format: 'csv',
        label: 'CSV',
        menuLabel: 'Download CSV',
        mimeType: 'text/csv',
        uti: 'public.comma-separated-values-text',
    },
]

export function exportFormatEntry(format: ExportFormat): ExportFormatEntry {
    const entry = EXPORT_FORMATS.find(e => e.format === format)
    if (!entry) throw new Error(`Unknown export format ${format}`)
    return entry
}

/** The run's options as an export sends them: no page, and the reader's display. */
export type ExportOptions = Omit<RunOptions, 'cursor' | 'limit' | 'display'> & {
    display: DisplayOptions
}

export interface ExportRequest {
    reportId: string
    params: Record<string, unknown>
    options: ExportOptions
    /** The reader's clock, for the file's date-time cells and its "made at" line. */
    hour12: boolean
}

type SavedRunBody = Extract<RunBody, { reportId: string }>

/**
 * The export request for the view's run body: the same params, folds, sort, day
 * and zone, the reader's display options and clock, and no paging (the server
 * reads every page itself).
 */
export function exportRequestOf(body: SavedRunBody, display: DisplayOptions): ExportRequest {
    const { cursor: _cursor, limit: _limit, display: _display, ...options } = body.options
    return {
        reportId: body.reportId,
        params: body.params,
        options: {
            ...options,
            // The view's run body leaves the zone to runReport, which adds it when it sends.
            timeZone: options.timeZone ?? readerTimeZone(),
            display,
        },
        hour12: readerHour12(),
    }
}

/** The server refused the export, or failed; the message is the server's when it gave one. */
export class ExportRefusal extends Error {
    readonly status: number

    constructor(status: number, message: string | undefined) {
        super(message || `The server answered ${status}.`)
        this.name = 'ExportRefusal'
        this.status = status
    }
}

function messageOf(body: unknown): string | undefined {
    if (!isRecord(body)) return undefined
    return typeof body.message === 'string' ? body.message : undefined
}

/**
 * Asks the server for the file, with the bearer token the route needs, and reads
 * its bytes (text would corrupt a PDF or an Excel file) and the name the server
 * gave it; `fallbackName` names it only when the header has no usable name.
 */
export async function fetchExport(
    request: ExportRequest,
    format: ExportFormat,
    fallbackName = ''
): Promise<{ bytes: Uint8Array<ArrayBuffer>; fileName: string }> {
    const headers: Record<string, string> = { 'Content-Type': 'application/json' }
    if (pb.authStore.token) headers.Authorization = `Bearer ${pb.authStore.token}`
    const response = await serverFetch(pb.buildURL(EXPORT_PATH), {
        method: 'POST',
        headers,
        body: JSON.stringify({ ...request, format }),
    })
    if (!response.ok) {
        const body: unknown = await response.json().catch(() => null)
        throw new ExportRefusal(response.status, messageOf(body))
    }
    const bytes = new Uint8Array(await response.arrayBuffer())
    const named = fileNameFromDisposition(response.headers.get('Content-Disposition'))
    return { bytes, fileName: named ?? (fallbackName || `report.${format}`) }
}

/** The most characters of the name part (Go maxFileNameRunes). */
const MAX_FILE_NAME_CHARS = 80
// Go unicode.IsSpace: the ASCII spaces, U+0085, U+00A0 and every Z character.
const SPACE = /[\t\n\v\f\r \u0085\p{Z}]/u
// Go unicode.IsLetter, IsMark and IsDigit (a decimal digit, Nd).
const KEPT = /[\p{L}\p{M}\p{Nd}]/u

/** The name part of exportFileName (Go fileNameStem). */
function fileNameStem(title: string): string {
    let stem = ''
    let dash = false
    for (const ch of title.toLowerCase()) {
        if (SPACE.test(ch) || ch === '-') {
            dash = true
        } else if (KEPT.test(ch)) {
            if (dash && stem !== '') stem += '-'
            dash = false
            stem += ch
        }
    }
    const cut = Array.from(stem).slice(0, MAX_FILE_NAME_CHARS).join('')
    const trimmed = cut.replace(/^-+|-+$/g, '')
    return trimmed === '' ? 'report' : trimmed
}

/** The date part of exportFileName (Go fileNameDate). */
function fileNameDate(
    defs: readonly ParamDef[],
    resolved: readonly ResolvedParam[],
    today: string
): string {
    const period = firstDateParam(defs)
    if (!period) return today
    const r = resolved.find(p => p.id === period.id)
    if (!r || r.unset) return today
    const day = typeof r.value === 'string' ? r.value : ''
    if (period.type === 'dateRange' && r.from && r.to) return `${r.from}-to-${r.to}`
    if (period.type === 'asOf' && isIsoDay(day)) return `as-of-${day}`
    if (period.type === 'date' && isIsoDay(day)) return day
    return today
}

/**
 * The file's name, as the server names it (Go exportFileName, same cases in
 * fixtures/format-cases.json): the title in lower case, words joined by "-",
 * every character but letters, marks, digits and "-" removed, at most 80
 * characters ("report" when nothing is left); then the period, the as-of day,
 * the single day, or the reader's day; then the format's extension.
 */
export function exportFileName(input: {
    title: string
    defs: readonly ParamDef[]
    resolved: readonly ResolvedParam[]
    today: string
    format: ExportFormat
}): string {
    const date = fileNameDate(input.defs, input.resolved, input.today)
    return `${fileNameStem(input.title)}-${date}.${input.format}`
}

const EXPORTABLE: ReadonlySet<ViewState> = new Set<ViewState>(['ready', 'empty'])

/** A report shown with rows, or with none (the file then holds its headings). */
export function canExport(state: ViewState): boolean {
    return EXPORTABLE.has(state)
}

/** Export is offered for a saved report in the main view, when it is not being edited. */
export function exportOffered(input: {
    isSaved: boolean
    isEditing: boolean
    isMainView: boolean
}): boolean {
    return input.isSaved && !input.isEditing && input.isMainView
}
