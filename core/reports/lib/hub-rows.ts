import { rolesByReport, type ShareRole } from './access'

// How the Reports hub turns its live queries into one list. The queries read rows
// under the reader's rules; these only merge, de-duplicate and order them.

interface HubRecord {
    id: string
    title: string
    source: string
    updated: string
}

function byNewestChange(a: HubRecord, b: HubRecord): number {
    return b.updated.localeCompare(a.updated) || a.id.localeCompare(b.id)
}

function byTitle(a: HubRecord, b: HubRecord): number {
    return a.title.localeCompare(b.title) || a.id.localeCompare(b.id)
}

const uniqueById = <R extends HubRecord>(reports: readonly R[]): R[] => [
    ...new Map(reports.map(report => [report.id, report])).values(),
]

/**
 * The reports shared with the reader, from their share rows: one row each, newest
 * change first, with every role they hold on it (a report can reach a reader through
 * a direct and a group share). A share whose report the store has not filed (or the
 * reader can no longer read) has no report and is left out.
 */
export function sharedReports<R extends HubRecord>(
    rows: readonly { role: ShareRole; report: R | undefined }[]
): { reports: R[]; roles: Map<string, ShareRole[]> } {
    const shared = rows.flatMap(row => (row.report ? [{ report: row.report, role: row.role }] : []))
    const roles = rolesByReport(shared.map(row => ({ report: row.report.id, role: row.role })))
    const reports = uniqueById(shared.map(row => row.report)).sort(byNewestChange)
    return { reports, roles }
}

/**
 * Every report of one package the reader can open (their own, shared with them,
 * published), once each, by title. `prefix` is the package's source prefix
 * (`books.`): a report belongs to the package whose source it reads.
 */
export function packageReports<R extends HubRecord>(input: {
    prefix: string
    mine: readonly R[]
    shared: readonly R[]
    published: readonly R[]
}): R[] {
    const all = [...input.mine, ...input.shared, ...input.published]
    return uniqueById(all.filter(report => report.source.startsWith(input.prefix))).sort(byTitle)
}
