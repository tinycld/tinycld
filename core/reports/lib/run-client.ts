import { isRecord } from '@tinycld/core/lib/errors'
import { pb } from '@tinycld/core/lib/pocketbase'
import { ClientResponseError } from 'pocketbase'
import type { ReportSpec } from './spec'
import type { CatalogSource, Problem, RunResult } from './types'

// The report endpoints (core/server/reports/endpoint.go, catalog.go). A run is a
// read sent by POST, because a draft spec does not fit in a URL. The refusals a
// person can cause (400, 403, 408, 422) come back as data, so the global query
// error handler does not report them; every other failure is thrown.

export const RUN_PATH = '/api/reports/run'
export const SOURCES_PATH = '/api/reports/sources'

export interface RunOptions {
    expand?: unknown[][]
    expandAll?: boolean
    cursor?: string
    limit?: number
    /** The reader's day, which presets resolve against. */
    today: string
}

export type RunBody =
    | { reportId: string; params: Record<string, unknown>; options: RunOptions }
    | { spec: ReportSpec; params: Record<string, unknown>; options: RunOptions }

export type RunRefusal =
    | { kind: 'invalid'; message: string; problems: Problem[] }
    | { kind: 'missing'; message: string; problems: Problem[] }
    | { kind: 'no-access' }
    | { kind: 'timeout' }

export type RunOutcome = { kind: 'ok'; result: RunResult } | RunRefusal

export function problemsOf(data: unknown): Problem[] {
    if (!isRecord(data)) return []
    const out: Problem[] = []
    for (const [path, problem] of Object.entries(data)) {
        if (
            isRecord(problem) &&
            typeof problem.code === 'string' &&
            typeof problem.message === 'string'
        ) {
            out.push({ path, code: problem.code, message: problem.message })
        }
    }
    return out.sort((a, b) => a.path.localeCompare(b.path))
}

export function refusalOf(error: unknown): RunRefusal | null {
    if (!(error instanceof ClientResponseError)) return null
    const body = isRecord(error.response) ? error.response : {}
    const message = typeof body.message === 'string' ? body.message : ''
    switch (error.status) {
        case 400:
            return { kind: 'invalid', message, problems: problemsOf(body.data) }
        case 422:
            return { kind: 'missing', message, problems: problemsOf(body.data) }
        case 403:
            return { kind: 'no-access' }
        case 408:
            return { kind: 'timeout' }
    }
    return null
}

/** The server could not be reached, or failed (5xx, including the read-only 503). */
export function isTransientRunError(error: unknown): boolean {
    return error instanceof ClientResponseError && (error.status === 0 || error.status >= 500)
}

export async function runReport(body: RunBody, signal?: AbortSignal): Promise<RunOutcome> {
    try {
        // requestKey null: two reports may run at once (a frozen tab, the builder's
        // preview); the SDK would otherwise cancel one for the other.
        const result = await pb.send<RunResult>(RUN_PATH, {
            method: 'POST',
            body,
            signal,
            requestKey: null,
        })
        return { kind: 'ok', result }
    } catch (error) {
        const refusal = refusalOf(error)
        if (refusal) return refusal
        throw error
    }
}

export async function fetchCatalog(signal?: AbortSignal): Promise<CatalogSource[]> {
    const res = await pb.send<{ sources: CatalogSource[] }>(SOURCES_PATH, {
        method: 'GET',
        signal,
        requestKey: null,
    })
    return res.sources
}
