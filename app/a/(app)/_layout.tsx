import { DemoFollowUpModal } from '@tinycld/core/components/DemoFollowUpModal'
import { DemoIntroModal } from '@tinycld/core/components/DemoIntroModal'
import { DocumentTitle } from '@tinycld/core/components/DocumentTitle'
import { HelpDrawer } from '@tinycld/core/components/help/HelpDrawer'
import { HelpSearchPalette } from '@tinycld/core/components/help/HelpSearchPalette'
import { NotifyContextSync } from '@tinycld/core/components/NotifyContextSync'
import { SearchPalette } from '@tinycld/core/components/search-palette/SearchPalette'
import { AuthGate } from '@tinycld/core/components/workspace/AuthGate'
import { ImportNotifier } from '@tinycld/core/components/workspace/ImportNotifier'
import { SkeletonLayout } from '@tinycld/core/components/workspace/SkeletonLayout'
import { WorkspaceLayout } from '@tinycld/core/components/workspace/WorkspaceLayout'
import { useAuth } from '@tinycld/core/lib/auth'
import { trace } from '@tinycld/core/lib/debug-trace'
import { useHelpSearchShortcut } from '@tinycld/core/lib/help/use-help-search-shortcut'
import { markNavMilestone } from '@tinycld/core/lib/nav-perf'
import { activeSlugFromPathname } from '@tinycld/core/lib/org-routes'
import { useWorkspaceStore } from '@tinycld/core/lib/stores/workspace-store'
import { useOrgInfo } from '@tinycld/core/lib/use-org-info'
import { OrgSlugProvider } from '@tinycld/core/lib/use-org-slug'
import { usePathname, useUnstableGlobalHref } from 'expo-router'
import { useEffect } from 'react'

export default function OrgLayout() {
    trace('OrgLayout mount')

    return (
        <OrgSlugProvider>
            <OrgLayoutInner />
        </OrgSlugProvider>
    )
}

function OrgLayoutInner() {
    const auth = useAuth({ throwIfAnon: false })
    const isReady = !auth.isInitializing && auth.isLoggedIn
    const { org } = useOrgInfo()
    // Bind ⌘/ globally so the help palette is reachable from any
    // org-scoped screen — not just package detail screens. The hook
    // is a no-op on native.
    useHelpSearchShortcut()

    if (!isReady) {
        return (
            <>
                <SkeletonLayout />
                {!auth.isInitializing && !auth.isLoggedIn && <AuthGate />}
            </>
        )
    }

    return (
        <>
            <DocumentTitle title={org?.name} includeOrg={false} />
            <ActivePkgSync />
            <ImportNotifier />
            <NotifyContextSync />
            <WorkspaceLayout isReady={isReady} />
            <DemoIntroModal />
            <DemoFollowUpModal />
            <HelpDrawer />
            <HelpSearchPalette />
            <SearchPalette />
        </>
    )
}

// App-owned areas whose rail entries always link to their root — we never
// record a lastPackageHref for these.
const NON_RESTORABLE_SLUGS = new Set(['settings', 'help'])

function ActivePkgSync() {
    const pathname = usePathname()
    // Full href incl. query (?folder=sent, ?label=…) — pathname alone drops
    // those, and mail/drive encode inner view state in query params.
    const href = useUnstableGlobalHref()

    useEffect(() => {
        const slug = activeSlugFromPathname(pathname)
        // settings/help are chrome, not a "current package" — they leave the
        // active-package highlight where it was.
        const nextSlug = slug === 'settings' || slug === 'help' ? null : slug
        if (nextSlug) markNavMilestone(nextSlug, 'route-committed')

        const state = useWorkspaceStore.getState()
        if (state.isDrawerOpen) state.setDrawerOpen(false)
        if (state.activePkgSlug !== nextSlug) state.setActivePkgSlug(nextSlug)

        // Record the exact inner route as the package's "return here" href, so
        // the rail (and MobileTabBar) reopen the view the user left — a thread,
        // a drive folder, a filtered mail folder — rather than resetting to the
        // list. The full href (path + query) is captured at any depth; a
        // package root (e.g. /mail) is itself a valid href, so leaving a
        // package on its list still restores to the list. Skipped for app-owned
        // areas (settings/help/admin), which the rail always links to their root.
        if (
            nextSlug &&
            !NON_RESTORABLE_SLUGS.has(nextSlug) &&
            state.lastPackageHref[nextSlug] !== href
        ) {
            state.setLastPackageHref(nextSlug, href)
        }
    }, [pathname, href])

    return null
}
