import { and, eq, like } from '@tanstack/db'
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
import { packageReports, sharedReports } from '../lib/hub-rows'
import { useReportUIStore } from '../stores/report-ui-store'
import { useReportCatalog } from './useReportCatalog'

export const HUB_TABS = [
    { id: 'mine', label: 'Mine' },
    { id: 'shared', label: 'Shared with me' },
    { id: 'published', label: 'Published' },
] as const
export type HubTab = (typeof HUB_TABS)[number]['id']

/**
 * What the hub lists: one tab, or (with `?package=<slug>`) every report of that
 * package the reader can open, in one list with no tabs (D2): a package's link
 * should not make its readers guess whether a report is theirs, shared or published.
 */
export type HubView = HubTab | 'package'

const PACKAGE_SLUG = /^[a-z][a-z0-9-]*$/

function tabOf(value: unknown): HubTab {
    return HUB_TABS.find(tab => tab.id === value)?.id ?? 'mine'
}

type QueryState = { isReady: boolean; isError: boolean }

function hubState(queries: readonly QueryState[]): 'loading' | 'error' | 'ready' {
    if (queries.some(query => query.isError)) return 'error'
    return queries.every(query => query.isReady) ? 'ready' : 'loading'
}

// A source key starts with its package's slug and a dot (`books.journal_lines`).
// The slug is letters, digits and dashes only, so it holds no LIKE wildcard.
const sourcesOf = (slug: string) => `${slug}.%`

function useMineQuery(view: HubView, packageSlug: string | null) {
    const [reportsCollection] = useStore('reports')
    const isOn = view === 'mine' || view === 'package'
    return useMyLiveQuery(
        (q, { userId }) =>
            isOn
                ? q
                      .from({ report: reportsCollection })
                      .where(({ report }) =>
                          packageSlug
                              ? and(
                                    eq(report.owner, userId),
                                    like(report.source, sourcesOf(packageSlug))
                                )
                              : eq(report.owner, userId)
                      )
                      .orderBy(({ report }) => report.updated, 'desc')
                : null,
        { queryKey: ['reports.hub.mine', view, packageSlug] }
    )
}

/** A saved report as the hub's queries read it. */
export type HubReport = NonNullable<ReturnType<typeof useMineQuery>['data']>[number]

export interface HubRow {
    report: HubReport
    access: ReportAccess
}

// The hub's lists. Only the queries the view needs run; each is a pbtsdb live query,
// so a report shared, published or deleted elsewhere appears or leaves at once. The
// package view reads the three lists, each filtered to the package, rather than one
// query over every report the reader may read: a removed share or an unpublish
// changes no report row the reader still sees, so only the list it came through can
// drop it.
export function useReportsHub() {
    const search = useLocalSearchParams<{ tab?: string; package?: string }>()
    const tab = tabOf(search.tab)
    const packageSlug =
        typeof search.package === 'string' && PACKAGE_SLUG.test(search.package)
            ? search.package
            : null
    const view: HubView = packageSlug ? 'package' : tab
    const { user } = useAuth()
    const role = useCurrentRole()
    const orgHref = useOrgHref()
    const catalog = useReportCatalog()
    const packages = usePackages()
    const startVisit = useReportUIStore(s => s.startVisit)
    const [reportsCollection, sharesCollection] = useStore('reports', 'report_shares')

    const mine = useMineQuery(view, packageSlug)
    const shared = useMyLiveQuery(
        (q, { userId }) =>
            view === 'shared' || view === 'package'
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
        { queryKey: ['reports.hub.shared', view] }
    )
    const published = useLiveQuery({
        queryKey: ['reports.hub.published', view, packageSlug],
        query: q =>
            view === 'published' || view === 'package'
                ? q
                      .from({ report: reportsCollection })
                      .where(({ report }) =>
                          packageSlug
                              ? and(
                                    eq(report.published, true),
                                    like(report.source, sourcesOf(packageSlug))
                                )
                              : eq(report.published, true)
                      )
                      .orderBy(({ report }) => report.title)
                : null,
    })
    // The Published tab's roles; the package view reads them from its shared rows.
    const myShares = useMyLiveQuery(
        (q, { userId }) =>
            view === 'published'
                ? q.from({ share: sharesCollection }).where(({ share }) => eq(share.user, userId))
                : null,
        { queryKey: ['reports.hub.my-shares', view] }
    )

    const rowsOf = (
        reports: readonly HubReport[],
        roles: ReadonlyMap<string, ShareRole[]>
    ): HubRow[] =>
        reports.map(report => ({
            report,
            access: reportAccess({
                owner: report.owner,
                published: report.published,
                shippedKey: report.shipped_key,
                userId: user.id,
                roles: roles.get(report.id) ?? [],
                isAdmin: role.isAdmin,
            }),
        }))

    const rowsOfView = (): HubRow[] => {
        if (view === 'mine') return rowsOf(mine.data ?? [], new Map())
        const fromShares = sharedReports(shared.data ?? [])
        if (view === 'shared') return rowsOf(fromShares.reports, fromShares.roles)
        if (view === 'package') {
            const reports = packageReports({
                prefix: `${packageSlug}.`,
                mine: mine.data ?? [],
                shared: fromShares.reports,
                published: published.data ?? [],
            })
            return rowsOf(reports, fromShares.roles)
        }
        const roles = rolesByReport(
            (myShares.data ?? []).map(share => ({ report: share.report, role: share.role }))
        )
        return rowsOf(published.data ?? [], roles)
    }

    const byTab = { mine: [mine], shared: [shared], published: [published] }
    const active = view === 'package' ? [mine, shared, published] : byTab[view]
    const prefix = packageSlug ? `${packageSlug}.` : null

    return {
        view,
        tab,
        setTab: (next: HubTab) => router.setParams({ tab: next === 'mine' ? undefined : next }),
        rows: rowsOfView(),
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
