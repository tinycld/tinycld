import { eq } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { useAuth } from '@tinycld/core/lib/auth'
import { log } from '@tinycld/core/lib/logger'
import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { usePackages } from '@tinycld/core/lib/packages/use-packages'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { recoverLiveQueries } from '@tinycld/core/lib/recover-live-queries'
import { useCurrentRole } from '@tinycld/core/lib/use-current-role'
import { useMyLiveQuery } from '@tinycld/core/lib/use-my-live-query'
import { router, useLocalSearchParams } from 'expo-router'
import { materialize } from 'pbtsdb'
import { type ReportAccess, reportAccess, rolesByReport, type ShareRole } from '../lib/access'
import { useReportUIStore } from '../stores/report-ui-store'
import { useReportCatalog } from './useReportCatalog'

export const HUB_TABS = [
    { id: 'mine', label: 'Mine' },
    { id: 'shared', label: 'Shared with me' },
    { id: 'published', label: 'Published' },
] as const
export type HubTab = (typeof HUB_TABS)[number]['id']

const PACKAGE_SLUG = /^[a-z][a-z0-9-]*$/

function tabOf(value: unknown): HubTab {
    return HUB_TABS.find(tab => tab.id === value)?.id ?? 'mine'
}

function hubState(query: { isReady: boolean; isError: boolean }): 'loading' | 'error' | 'ready' {
    if (query.isError) return 'error'
    return query.isReady ? 'ready' : 'loading'
}

function useMineQuery(tab: HubTab) {
    const [reportsCollection] = useStore('reports')
    return useMyLiveQuery(
        (q, { userId }) =>
            tab === 'mine'
                ? q
                      .from({ report: reportsCollection })
                      .where(({ report }) => eq(report.owner, userId))
                      .orderBy(({ report }) => report.updated, 'desc')
                : null,
        { queryKey: ['reports.hub.mine', tab] }
    )
}

/** A saved report as the hub's queries read it. */
export type HubReport = NonNullable<ReturnType<typeof useMineQuery>['data']>[number]

function byNewestChange(a: HubReport, b: HubReport): number {
    return b.updated.localeCompare(a.updated) || a.id.localeCompare(b.id)
}

export interface HubRow {
    report: HubReport
    access: ReportAccess
}

// The hub's three lists. Only the open tab's query runs; each is a pbtsdb live query,
// so a report shared, published or deleted elsewhere appears or leaves at once.
export function useReportsHub() {
    const search = useLocalSearchParams<{ tab?: string; package?: string }>()
    const tab = tabOf(search.tab)
    const packageSlug =
        typeof search.package === 'string' && PACKAGE_SLUG.test(search.package)
            ? search.package
            : null
    const { user } = useAuth()
    const role = useCurrentRole()
    const orgHref = useOrgHref()
    const catalog = useReportCatalog()
    const packages = usePackages()
    const startVisit = useReportUIStore(s => s.startVisit)
    const [reportsCollection, sharesCollection] = useStore('reports', 'report_shares')

    const mine = useMineQuery(tab)
    const shared = useMyLiveQuery(
        (q, { userId }) =>
            tab === 'shared'
                ? q
                      .from({ share: sharesCollection })
                      .where(({ share }) => eq(share.user, userId))
                      .select(({ share }) => ({
                          role: share.role,
                          report: materialize(
                              q
                                  .from({ report: reportsCollection })
                                  .where(({ report }) => eq(report.id, share.report))
                                  .findOne()
                          ),
                      }))
                : null,
        { queryKey: ['reports.hub.shared', tab] }
    )
    const published = useLiveQuery({
        queryKey: ['reports.hub.published', tab],
        query: q =>
            tab === 'published'
                ? q
                      .from({ report: reportsCollection })
                      .where(({ report }) => eq(report.published, true))
                      .orderBy(({ report }) => report.title)
                : null,
    })
    const myShares = useMyLiveQuery(
        (q, { userId }) =>
            tab === 'published'
                ? q.from({ share: sharesCollection }).where(({ share }) => eq(share.user, userId))
                : null,
        { queryKey: ['reports.hub.my-shares', tab] }
    )

    const accessOf = (report: HubReport, roles: readonly ShareRole[]) =>
        reportAccess({
            owner: report.owner,
            published: report.published,
            shippedKey: report.shipped_key,
            userId: user.id,
            roles,
            isAdmin: role.isAdmin,
        })

    const rowsOfTab = (): HubRow[] => {
        if (tab === 'mine') {
            return (mine.data ?? []).map(report => ({ report, access: accessOf(report, []) }))
        }
        if (tab === 'shared') {
            const sharedRows = (shared.data ?? []).flatMap(row =>
                row.report ? [{ report: row.report, role: row.role }] : []
            )
            // A report can reach a reader through a direct and a group share: one row each.
            const roles = rolesByReport(
                sharedRows.map(row => ({ report: row.report.id, role: row.role }))
            )
            // Newest change first, as Mine; the report is a related row, so the order is
            // set here, after the query.
            const reports = [
                ...new Map(sharedRows.map(row => [row.report.id, row.report])).values(),
            ].sort(byNewestChange)
            return reports.map(report => ({
                report,
                access: accessOf(report, roles.get(report.id) ?? []),
            }))
        }
        const roles = rolesByReport(
            (myShares.data ?? []).map(share => ({ report: share.report, role: share.role }))
        )
        return (published.data ?? []).map(report => ({
            report,
            access: accessOf(report, roles.get(report.id) ?? []),
        }))
    }

    const prefix = packageSlug ? `${packageSlug}.` : null
    const all = rowsOfTab()
    const rows = prefix ? all.filter(row => row.report.source.startsWith(prefix)) : all
    const active = { mine, shared, published }[tab]

    return {
        tab,
        setTab: (next: HubTab) => router.setParams({ tab: next === 'mine' ? undefined : next }),
        rows,
        state: hubState(active),
        retry: () => {
            void recoverLiveQueries({
                queries: [
                    mine.collection,
                    shared.collection,
                    published.collection,
                    myShares.collection,
                ],
                sources: [reportsCollection, sharesCollection],
                onError: error => log.error('reports.hub', error),
            })
        },
        sources: prefix
            ? catalog.sources.filter(source => source.key.startsWith(prefix))
            : catalog.sources,
        isCatalogReady: catalog.isReady,
        isCatalogError: catalog.isError,
        retryCatalog: catalog.retry,
        sourceLabel: (key: string) => catalog.byKey.get(key)?.label ?? key,
        packageSlug,
        packageLabel: packages.find(p => p.slug === packageSlug)?.name ?? packageSlug ?? '',
        onShowAll: () => router.setParams({ package: undefined }),
        onOpen: (report: HubReport) => {
            startVisit()
            router.push(orgHref('reports/[id]', { id: report.id }))
        },
        onNew: (sourceKey: string) => router.push(orgHref('reports/new', { source: sourceKey })),
    }
}

export type ReportsHubModel = ReturnType<typeof useReportsHub>
